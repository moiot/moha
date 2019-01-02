FROM postgres:10.0
COPY 10-config.sh /docker-entrypoint-initdb.d/
COPY 20-replication.sh /docker-entrypoint-initdb.d/
COPY postgresql.conf /var/lib/
RUN chmod 777 /var/lib/postgresql.conf
COPY supervise /agent/supervise
COPY mysql-agent /agent/mysql-agent
COPY mysql-agent-tear-down /agent/mysql-agent-tear-down
COPY mysql-agent-service-boot /agent/mysql-agent-service-boot
COPY mysql-agent-service-boot-daemon /agent/mysql-agent-service-boot-daemon