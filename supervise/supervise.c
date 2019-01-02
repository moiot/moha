// Copyright 2018 MOBIKE, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

#include <sys/eventfd.h>
#include <sys/wait.h>
#include <sys/types.h>
#include <unistd.h>
#include <stdlib.h>
#include <stdio.h>
#include <stdint.h>             /* Definition of uint64_t */
#include <signal.h>

#include <errno.h>
#include <sys/epoll.h>

#include "log.h"

#define handle_error(msg) \
    do { perror(msg); exit(EXIT_FAILURE); } while (0)

// TODO(shangliang) change name
#define CHILD_PROCESS "/agent/mysql-agent"
#define TEAR_DOWN_PROCESS "/agent/mysql-agent-tear-down"
#define CHILD_INIT_WAIT 60000  // epoll timeout has "MILLISECOND" as time unit
#define EPOLL_TIMEOUT 5000  // epoll timeout has "MILLISECOND" as time unit
#define FD_PARAM 10
#define PASSWORD_PARAM 128

int _epollfd, _eventfd, _agentpid, _servicepid = 0;
uint64_t _u;

int init_logger() {
    log_set_filename("/agent/log/supervise.log");
    log_set_level(2);
    log_init();
}

int init() {
    init_logger();
    _epollfd = epoll_create1(0);
    if (_epollfd == -1)
        handle_error("epollfd");
    _eventfd = eventfd(0, EFD_NONBLOCK);
    if (_eventfd == -1)
        handle_error("eventfd");
    log_info("_eventfd is %d", _eventfd);
    // TODO(shangliang) 初始化成无效值
    struct epoll_event evnt = {0};
    evnt.data.fd = _eventfd;
    evnt.events = EPOLLIN | EPOLLET;
    if (epoll_ctl(_epollfd, EPOLL_CTL_ADD, _eventfd, &evnt) == -1)
        // TODO(shangliang) add log
        abort();
}

int destroy() {
    log_info("begin to destroy!");
    close(_eventfd);
    close(_epollfd);

    // shutdown mysql
    int ret = fork();
    if (ret == 0) {
        // shutdown mysql
        // /usr/bin/mysqladmin
        char* root_password = getenv("MYSQL_ROOT_PASSWORD");
        char password_param[PASSWORD_PARAM];
        snprintf(password_param, PASSWORD_PARAM, "-p%s", root_password);
        char* socket = getenv("MYSQL_SOCKET");
        char socket_param[PASSWORD_PARAM];
        snprintf(socket_param, PASSWORD_PARAM, "-S%s", socket);

        log_debug("password_param is %s", password_param);
        log_debug("socket_param is %s", socket_param);
        execl("/usr/bin/mysqladmin",
              "/usr/bin/mysqladmin",
              "-uroot", password_param, socket_param, "shutdown",
                (char*) NULL);
    } else {
        // run tear-down hook
        execl(TEAR_DOWN_PROCESS,
                TEAR_DOWN_PROCESS, "-config=/agent/config.toml",
                (char*) NULL);
    }
}

int wait_child_pid(int* status);
int handle_child_exit(int status);

static void sig_handler_exit(int sig) {
    log_info("get signal %d, shutdown gracefully", sig);
    destroy();
    exit(0);
}

static void sig_handler_ignore(int sig) {
    log_info("get signal %d, just ignore", sig);
}

static void sig_handler_child(int sig) {
    log_info("get signal %d ", sig);
    int status;
    log_info("wait child pid from sig handler");
    int r = wait_child_pid(&status);
    log_info("wait child pid from sig handler return result %d and status %d",
        r, WEXITSTATUS(status));
    if (r == 2) {
        destroy();
        return;
    } else if (r == 1) {
        log_info("handle child exit from sig handler");
        if (handle_child_exit(status) < 0) {
            destroy();
        }
    } else {
        log_info("ignore child exit from sig handler");
    }
}

void register_signal_handler() {
    // TODO(shangliang) SIGHUP ignore
    if (signal(SIGHUP, sig_handler_ignore) == SIG_ERR)
        handle_error("cannot catch SIGHUP");
    if (signal(SIGINT, sig_handler_exit) == SIG_ERR)
        handle_error("cannot catch SIGINT");
    if (signal(SIGTERM, sig_handler_exit) == SIG_ERR)
        handle_error("cannot catch SIGTERM");
    if (signal(SIGQUIT, sig_handler_exit) == SIG_ERR)
        handle_error("cannot catch SIGQUIT");
    // TODO(shangliang) SIGPIPE  ignore
    // TODO(shangliang) SIGCHLD handle mysql or Agent down
    if (signal(SIGCHLD, sig_handler_child) == SIG_ERR)
            handle_error("cannot catch SIGCHLD");
    return;
}

void run_child() {
    log_info("child process writes to fd %d", _eventfd);
    char str[FD_PARAM];
    snprintf(str, FD_PARAM, "-fd=%d", _eventfd);
    execl(CHILD_PROCESS,
        CHILD_PROCESS, "-config=/agent/config.toml", str,
        (char*) NULL);
}

int handle_child_exit(int status) {
    int exit_status = WEXITSTATUS(status);
    log_info("child %d exited with status %d, WIFEXITED(status) is %d",
        _agentpid, exit_status, WIFEXITED(status));
    if (WIFEXITED(status) && (exit_status == 0 || exit_status == 3)) {
        log_info("child exits as expected with return code %d, "\
            "supervise exit as well",
            exit_status);
        return -1;
    }
    log_info("try to restart %s", CHILD_PROCESS);
    // do not handle mysql startup issue
    // just consider Agent exits while MySQL works fine
    int ret = fork();
    if (ret == 0) {
        run_child();
    } else {
        _agentpid = ret;
    }
    return 0;
}

// wait_child_pid gets the child processes running status
// return 0 if _agentpid is still alive
// return -1 id error occurs
// return 1 if child process of pid _agentpid exits
// return 2 if adopted service process exits
// return 3 if any other process exits
int wait_child_pid(int* status) {
    while (1) {
        int cpid = waitpid(-1, status, WNOHANG);
        if (cpid == 0) {
            // Child still alive
            log_info("child is still alive, from wait_child_pid");
            return 0;
        } else if (cpid == -1) {
            // Error
            log_error("waitpid error, errno is %d", errno);
            return -1;
        } else if (cpid == _agentpid) {
            // Child exited
            log_info("receive child %d exits, %s", cpid, CHILD_PROCESS);
            return 1;
        } else if (cpid == _servicepid) {
            // Child exitedsupervise/supervise.c:121
            log_info("receive child %d exits, service", cpid);
            return 2;
        } else {
            log_info("receive child %d exits, not %s nor service. ignore",
                cpid, CHILD_PROCESS);
            return 3;
        }
    }
}

// detect_child_alive detects whether the child is still alive,
// returns 1 if the child is still alive
// return 0 if timeout
// retry if interrupted
// return -2 for other errors
int detect_child_alive(int epoll_timeout) {
    // epoll
    static const int EVENTS = 1;
    struct epoll_event evnts[EVENTS];
    int count = epoll_wait(_epollfd, evnts, EVENTS, epoll_timeout);
    log_info("epoll_wait returns %d count", count);

    int i;
    for (i = 0; i < count; ++i) {
        struct epoll_event *e = evnts + i;
        if (e->data.fd == _eventfd) {
            eventfd_t val;
            eventfd_read(_eventfd, &val);
            // log_info("receive eventfd value: %lld\n", (long long)val);
            if (val > 1) {
                int spid = (uint64_t)val>>32;
                if (spid > 0) {
                    _servicepid = spid;
                    log_info(" receive eventfd, with service pid: %lld\n",
                        _servicepid);
                }
            }
            break;
        }
    }

    if (count == -1) {
        if (errno != EINTR) {
            log_error("epoll_wait error with errno: %d", errno);
            return -2;
        } else {
            log_info("receive interrupt, retry");
            // interrupt is SIGCHLD, so detect child status with short timeout
            return detect_child_alive(epoll_timeout/2 +1);
        }
    } else if (count == 0) {
        log_error("timeout to wait child");
        return 0;
    }
    return 1;
}

void run_server() {
    while (1) {
        int alive_result = detect_child_alive(EPOLL_TIMEOUT);
        if (alive_result > 0) {
            log_info("%s is still alive, from detect_child_alive",
                CHILD_PROCESS);
            continue;
        } else {
            // NOW the child might stop
            int status;
            int r = wait_child_pid(&status);
            if (r == 0) {
                log_info("%s is still alive, from wait_child_pid, maybe hung",
                    CHILD_PROCESS);
                log_info("supervise exit");
                return;
            } else if (r == 2) {
                destroy();
                // continue;
            } else if (r > 0) {
                log_info("%s exits, from wait_child_pid", CHILD_PROCESS);
                if (handle_child_exit(status) < 0) {
                    return;
                }
            } else {
                log_info("wait_child_pid error");
                return;
            }
        }
    }
}

int main(int argc, char *argv[]) {
    init();
    register_signal_handler();

    int ret = fork();
    if (ret == 0) {
        run_child();
    } else {
        _agentpid = ret;
        if (detect_child_alive(CHILD_INIT_WAIT) < 0) {
            destroy();
            return 0;
        }
        run_server();
        destroy();
    }
}
