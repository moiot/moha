FROM postgres:10.0
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod 777 /usr/local/bin/docker-entrypoint.sh
COPY 10-config.sh /docker-entrypoint-initdb.d/
COPY supervise /agent/supervise
COPY mysql-agent /agent/mysql-agent
COPY mysql-agent-tear-down /agent/mysql-agent-tear-down
COPY mysql-agent-service-boot /agent/mysql-agent-service-boot
COPY mysql-agent-service-boot-daemon /agent/mysql-agent-service-boot-daemon