MYSQL_REPLICATION_USER='replication'
MYSQL_REPLICATION_PASSWORD='replication_pwd'
MYSQL_MASTER_ROOT_PASSWORD='master_root_pwd'
MYSQL_NODE_ROOT_PASSWORD='master_root_pwd'

# set master read only 
mysql -h 127.0.0.1 -P 3306 -u root -p${MYSQL_MASTER_ROOT_PASSWORD} \
-e "FLUSH TABLES WITH READ LOCK; \
    SET GLOBAL read_only = 1;"

# promote new master
mysql -h 127.0.0.1 -P 3307 -u root -p${MYSQL_NODE_ROOT_PASSWORD} \
-e " stop slave; \
     reset master; \
     GRANT REPLICATION SLAVE ON *.* TO 'replication'@'%';"

# old master set to slave 
mysql -h 127.0.0.1 -P 3306 -u root -p${MYSQL_MASTER_ROOT_PASSWORD} \
-e " SET GLOBAL read_only = 0; \
     UNLOCK TABLES; \
      stop slave ; \
      change master to master_user='replication',master_host='mysql-node-1',master_password='replication_pwd'; \
     start slave;"


# other nodes master redirect 
mysql -h 127.0.0.1 -P 3308 -u root -p${MYSQL_NODE_ROOT_PASSWORD} \
-e "stop slave ; \
    change master to master_user='replication',master_host='mysql-node-1',master_password='replication_pwd'; \
    start slave;"