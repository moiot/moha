/*
 * Copyright (c) 2017 rxi
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to
 * deal in the Software without restriction, including without limitation the
 * rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
 * sell copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
 * FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
 * IN THE SOFTWARE.
 */

#include <stdio.h>
#include <stdlib.h>
#include <stdarg.h>
#include <string.h>
#include <time.h>

#include "log.h"

static struct {
  void *udata;
  log_LockFn lock;
  char *filename;
  FILE *fp;
  int size;
  int max_size;
  int level;
  int quiet;
} L;


static const char *level_names[] = {
  "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"
};

#ifdef LOG_USE_COLOR
static const char *level_colors[] = {
  "\x1b[94m", "\x1b[36m", "\x1b[32m", "\x1b[33m", "\x1b[31m", "\x1b[35m"
};
#endif


static void lock(void)   {
  if (L.lock) {
    L.lock(L.udata, 1);
  }
}


static void unlock(void) {
  if (L.lock) {
    L.lock(L.udata, 0);
  }
}


void log_set_udata(void *udata) {
  L.udata = udata;
}


void log_set_lock(log_LockFn fn) {
  L.lock = fn;
}


void log_set_fp(FILE *fp) {
  L.fp = fp;
}

void log_set_filename(char *filename) {
  L.filename = filename;
}


void log_set_level(int level) {
  L.level = level;
}


void log_set_quiet(int enable) {
  L.quiet = enable ? 1 : 0;
}

void log_init() {
  // create file if non-exist
  fclose(fopen(L.filename, "a"));
  FILE *stream = fopen(L.filename, "r+");
  log_set_fp(stream);
  fseek(stream, 0 , SEEK_END);
  L.size = ftell(stream);
  L.max_size = 300 * 1024 * 1024;  // nearly 300MB
}

void rotate();
void log_log(int level, const char *file, int line, const char *fmt, ...) {
  if (level < L.level) {
    return;
  }

  /* Acquire lock */
  lock();

  /* Get current time */
  time_t t = time(NULL);
  struct tm lt;
  localtime_r(&t, &lt);

  /* Log to stderr */
  if (!L.quiet) {
    va_list args;
    char buf[16];
    buf[strftime(buf, sizeof(buf), "%H:%M:%S", &lt)] = '\0';
#ifdef LOG_USE_COLOR
    fprintf(
      stderr, "%s %s%-5s\x1b[0m \x1b[90m%s:%d:\x1b[0m ",
      buf, level_colors[level], level_names[level], file, line);
#else
    fprintf(stderr, "%s %-5s %s:%d: ", buf, level_names[level], file, line);
#endif
    va_start(args, fmt);
    vfprintf(stderr, fmt, args);
    va_end(args);
    fprintf(stderr, "\n");
  }

  /* Log to file */
  if (L.fp) {
    va_list args;
    char buf[32];
    buf[strftime(buf, sizeof(buf), "%Y-%m-%d %H:%M:%S", &lt)] = '\0';
    int n = fprintf(L.fp, "%s %-5s %s:%d: ",
                        buf, level_names[level], file, line);
    va_start(args, fmt);
    n += vfprintf(L.fp, fmt, args);
    va_end(args);
    n += fprintf(L.fp, "\n");
    L.size += n;
    fflush(L.fp);
    if (L.size > L.max_size) {
      rotate();
      L.size = 0;
    }
  }

  /* Release lock */
  unlock();
}

void log_close(FILE *fp) {
  fclose(fp);
}

void current_time(char* buffer, int size) {
  time_t rawtime;
  struct tm timeinfo;
  time(&rawtime);
  localtime_r(&rawtime, &timeinfo);
  snprintf(buffer, size, "%d-%02d-%02dT%02d-%02d-%02d",
           timeinfo.tm_year+1900,
           timeinfo.tm_mon+1,
           timeinfo.tm_mday,
           timeinfo.tm_hour,
           timeinfo.tm_min,
           timeinfo.tm_sec);
  return;
}

void log_rename(char* filename) {
  char new_filename[1024];
  char date_str[20];
  current_time(date_str, 20);
  snprintf(new_filename, sizeof(new_filename), "%s.%s", filename, date_str);
  rename(filename, new_filename);
}

void rotate() {
  log_close(L.fp);
  log_rename(L.filename);
  FILE *stream = fopen(L.filename, "a");
  log_set_fp(stream);
}
