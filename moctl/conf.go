package main

import (
	"flag"
	"fmt"
	"logger"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	defaultName = "conf.toml"
)

// conf
type AgentConfig struct {
	*flag.FlagSet
	MysqlConf  MysqlConf  `toml:"mysql" json:"mysql"`
	DockerConf DockerConf `toml:"docker" json:"docker"`
	AgentConf  AgentConf  `toml:"agent" json:"agent"`
	configFile string
}

type MysqlConf struct {
	HostAddress  string `toml:"hostaddress" json:"hostaddress"`
	Port         string `toml:"port" json:"port"`
	InstanceName string `toml:"instancename" json:"instancename"`
	BufferPool   string `toml:"bufferpool" json:"bufferpool"`
	SQLMode      string `toml:"sqlmode" json:"sqlmode"`
	DataDir      string `toml:"datadir" json:"datadir"`
}

type AgentConf struct {
	HostAddress     string `toml:"hostaddress" json:"hostaddress"`
	Port            string `toml:"port" json:"port"`
	InstanceName    string `toml:"instancename" json:"instancename"`
	ProxyName       string `toml:"proxyname" json:"proxyname"`
	EtcdURL         string `toml:"etcdurl" json:"etcdurl"`
	EtcdUser        string `toml:"etcduser" json:"etcduser"`
	EtcdPasswd      string `toml:"etcdpasswd" json:"etcdpasswd"`
	AgentUser       string `toml:"agentuser" json:"agentuser"`
	AgentPasswd     string `toml:"agentpasswd"  json:"agentpasswd"`
	MysqlReplUser   string `toml:"mysqlrepluser" json:"mysqlrepluser"`
	MysqlReplPasswd string `toml:"mysqlreplpasswd" json:"mysqlreplpasswd"`
	DataDir         string `toml:"datador" json:"datadir"`
}

type DockerConf struct {
	Port            string `toml:"port" json:"port"`
	InstanceName    string `toml:"instancename" json:"instancename"`
	Version         string `toml:"version" json:"version"`
	DataDir         string `toml:"datadir" json:"datadir"`
	AgentUser       string `toml:"agentuser" json:"agentuser"`
	AgentPasswd     string `toml:"agentpasswd"  json:"agentpasswd"`
	MysqlReplUser   string `toml:"mysqlrepluser" json:"mysqlrepluser"`
	MysqlReplPasswd string `toml:"mysqlreplpasswd" json:"mysqlreplpasswd"`
	MysqlRootPasswd string `toml:"mysqlrootpasswd" json:"mysqlrootpasswd"`
}

type CreateDockerCfg struct {
	ComposeVersion string                  `yaml:"version"`
	Services       map[string]ServicesName `yaml:"services"`
}

type ServicesName struct {
	Image         string      `yaml:"image"`
	NetworkMode   string      `yaml:"network_mode"`
	EntryPoint    string      `yaml:"entrypoint"`
	Volumes       []string    `yaml:"volumes"`
	ContainerName string      `yaml:"container_name"`
	Environment   Environment `yaml:"environment"`
}

type Environment struct {
	MySQLDatabase   string `yaml:"MYSQL_DATABASE"`
	MysqlPasswd     string `yaml:"MYSQL_PASSWORD"`
	MysqlReplPasswd string `yaml:"MYSQL_REPLICATION_PASSWORD"`
	MysqlReplUser   string `yaml:"MYSQL_REPLICATION_USER"`
	MysqlRootPasswd string `yaml:"MYSQL_ROOT_PASSWORD"`
	MysqlUser       string `yaml:"MYSQL_USER"`
	MysqlSocket     string `yaml:"MYSQL_SOCKET"`
}

type Config struct {
	*flag.FlagSet
	ClusterName           string   `toml:"cluster-name" json:"cluster-name"`
	LeaderLeaseTTL        int64    `toml:"leader-lease-ttl" json:"leader-lease-ttl"`
	ShutdownThreshold     int64    `toml:"shutdown-threshold" json:"shutdown-threshold"`
	DataDir               string   `toml:"data-dir" json:"data-dir"`
	ListenAddr            string   `toml:"addr" json:"addr"`
	EtcdURLs              string   `toml:"etcd-urls" json:"etcd-urls"`
	RegisterTTL           int      `toml:"register-ttl" json:"register-ttl"`
	EtcdRootPath          string   `toml:"etcd-root-path" json:"etcd-root-path"`
	EtcdUsername          string   `toml:"etcd-username" json:"etcd-username"`
	EtcdPassword          string   `toml:"etcd-password" json:"etcd-password"`
	LogLevel              string   `toml:"log-level" json:"log-level"`
	LogFile               string   `toml:"log-file" json:"log-file"`
	LogMaxSize            int      `toml:"log-max-size" json:"log-max-size"`
	LogMaxDays            int      `toml:"log-max-days" json:"log-max-days"`
	LogCompress           bool     `toml:"log-compress" json:"log-compress"`
	DBConfig              DBConfig `toml:"db-config" json:"db-config"`
	InternalServiceHost   string   `toml:"internal-service-host" json:"internal-service-host"`
	ExternalServiceHost   string   `toml:"external-service-host" json:"external-service-host"`
	ForkProcessFile       string   `toml:"fork-process-file" json:"fork-process-file"`
	ForkProcessArgs       []string `toml:"fork-process-args" json:"fork-process-args"`
	ForkProcessWaitSecond int      `toml:"fork-process-wait-second" json:"fork-process-wait-second"`
}

// DBConfig is the DB configuration.
type DBConfig struct {
	Host                string `toml:"host" json:"host"`
	User                string `toml:"user" json:"user"`
	Password            string `toml:"password" json:"password"`
	Port                int    `toml:"port" json:"port"`
	Timeout             string `toml:"timeout" json:"timeout"`
	ReplicationUser     string `toml:"replication_user" json:"replication_user"`
	ReplicationPassword string `toml:"replication_password" json:"replication_password"`
	ReplicationNet      string `toml:"replication_net" json:"replication_net"`
}

type MySQLCfg struct {
	MySQLPara MySQLPara `toml:"mysqld" json:"mysqld"`
}

type MySQLPara struct {
	User                              string `toml:"user" json:"user"`
	SQLMode                           string `toml:"sql_mode" json:"sql_mode"`
	AutoCommit                        int    `toml:"autocommit" json:"autocommit"`
	InitConnect                       string `toml:"init_connect" json:"init_connect"`
	CharacterSetServer                string `toml:"character_set_server" json:"character_set_server"`
	CollationServer                   string `toml:"collation-server" json:"collation-server"`
	EventScheduler                    int    `toml:"event_scheduler" json:"event_scheduler"`
	MaxAllowedPacket                  string `toml:"max_allowed_packet" json:"max_allowed_packet"`
	LowerCaseTableNames               int    `toml:"lower_case_table_names" json:"lower_case_table_names"`
	BindAddress                       string `toml:"bind-address" json:"bind-address"`
	ReportHost                        string `toml:"report_host" json:"report_host"`
	ServerID                          int    `toml:"server-id" json:"server-id"`
	Port                              int    `toml:"port" json:"port"`
	Socket                            string `toml:"socket" json:"socket"`
	PidFile                           string `toml:"pid-file" json:"pid-file"`
	DataDir                           string `toml:"datadir" json:"datadir"`
	SkipSymbolicLinks                 int    `toml:"skip-symbolic-links" json:"skip-symbolic-links"`
	ShowCompatibility56               int    `toml:"show_compatibility_56" json:"show_compatibility_56"`
	ExplicitDefaultsForTimestamp      int    `toml:"explicit_defaults_for_timestamp" json:"explicit_defaults_for_timestamp"`
	DefaultStorageEngine              string `toml:"default-storage-engine" json:"default-storage-engine"`
	TransactionIsolation              string `toml:"transaction_isolation" json:"transaction_isolation"`
	ReadOnly                          int    `toml:"read_only" json:"read_only"`
	SkipNameResolve                   int    `toml:"skip-name-resolve" json:"skip-name-resolve"`
	WaitTimeout                       int    `toml:"wait_timeout" json:"wait_timeout"`
	InteractiveTimeout                int    `toml:"interactive_timeout" json:"interactive_timeout"`
	LockWaitTimeout                   int    `toml:"lock_wait_timeout" json:"lock_wait_timeout"`
	MaxConnectErrors                  int    `toml:"max_connect_errors" json:"max_connect_errors"`
	MaxConnections                    int    `toml:"max_connections" json:"max_connections"`
	BinlogFormat                      string `toml:"binlog_format" json:"binlog_format"`
	BinlogGtidGimpleRecovery          int    `toml:"binlog_gtid_simple_recovery" json:"binlog_gtid_simple_recovery"`
	BinlogRowsQueryLogEvents          int    `toml:"binlog_rows_query_log_events" json:"binlog_rows_query_log_events"`
	ExpireLogsDays                    int    `toml:"expire_logs_days" json:"expire_logs_days"`
	BinlogRowImage                    string `toml:"binlog_row_image" json:"binlog_row_image"`
	MaxBinlogSize                     string `toml:"max_binlog_size" json:"max_binlog_size"`
	BackLog                           int    `toml:"back_log" json:"back_log"`
	GeneralLogFile                    string `toml:"general_log_file" json:"general_log_file"`
	LogBin                            string `toml:"log-bin" json:"log-bin"`
	LogError                          string `toml:"log-error" json:"log-error"`
	LogQueriesNotUsingIndexes         int    `toml:"log_queries_not_using_indexes" json:"log_queries_not_using_indexes"`
	LogSlaveUpdates                   int    `toml:"log_slave_updates" json:"log_slave_updates"`
	LogSlowAdminStatements            int    `toml:"log_slow_admin_statements" json:"log_slow_admin_statements"`
	LogSlowSlaveStatements            int    `toml:"log_slow_slave_statements" json:"log_slow_slave_statements"`
	LogThrottleQueriesNotUsingIndexes int    `toml:"log_throttle_queries_not_using_indexes" json:"log_throttle_queries_not_using_indexes"`
	LogTimestamps                     string `toml:"log_timestamps" json:"log_timestamps"`
	LongQueryTime                     int    `toml:"long_query_time" json:"long_query_time"`
	MasterInfoRepository              string `toml:"master_info_repository" json:"master_info_repository"`
	RelayLogInfoRepository            string `toml:"relay_log_info_repository" json:"relay_log_info_repository"`
	RelayLogRecovery                  int    `toml:"relay_log_recovery" json:"relay_log_recovery"`
	RelayLog                          string `toml:"relay-log" json:"relay-log"`
	SlowQueryLog                      int    `toml:"slow_query_log" json:"slow_query_log"`
	SlowQueryLogFile                  string `toml:"slow_query_log_file" json:"slow_query_log_file"`
	SyncBinlog                        int    `toml:"sync_binlog" json:"sync_binlog"`
	SecureFilePriv                    string `toml:"secure_file_priv" json:"secure_file_priv"`
	EnforceGtidConsistency            int    `toml:"enforce-gtid-consistency" json:"enforce-gtid-consistency"`
	JoinBufferSize                    string `toml:"join_buffer_size" json:"join_buffer_size"`
	KeyBufferSize                     string `toml:"key_buffer_size" json:"key_buffer_size"`
	MinExaminedRowLimit               int    `toml:"min_examined_row_limit" json:"min_examined_row_limit"`
	ReadBufferSize                    string `toml:"read_buffer_size" json:"read_buffer_size"`
	ReadRndBufferSize                 string `toml:"read_rnd_buffer_size" json:"read_rnd_buffer_size"`
	SortBufferSize                    string `toml:"sort_buffer_size" json:"sort_buffer_size"`
	TableDefinitionCache              int    `toml:"table_definition_cache" json:"table_definition_cache"`
	TableOpenCache                    int    `toml:"table_open_cache" json:"table_open_cache"`
	TableOpenCacheInstances           int    `toml:"table_open_cache_instances" json:"table_open_cache_instances"`
	ThreadCacheSize                   int    `toml:"thread_cache_size" json:"thread_cache_size"`
	TmpTableSize                      string `toml:"tmp_table_size" json:"tmp_table_size"`
	GtidMode                          string `toml:"gtid_mode" json:"gtid_mode"`
	SkipSlaveStart                    int    `toml:"skip-slave-start" json:"skip-slave-start"`
	SlaveParallelType                 string `toml:"slave-parallel-type" json:"slave-parallel-type"`
	SlaveParallelWorkers              int    `toml:"slave-parallel-workers" json:"slave-parallel-workers"`
	SlavePreserveCommitOrder          int    `toml:"slave_preserve_commit_order" json:"slave_preserve_commit_order"`
	SlaveRowsSearchAlgorithms         string `toml:"slave-rows-search-algorithms" json:"slave-rows-search-algorithms"`
	SlaveTransactionRetries           int    `toml:"slave_transaction_retries" json:"slave_transaction_retries"`
	InnodbAutoincLockMode             int    `toml:"innodb_autoinc_lock_mode" json:"innodb_autoinc_lock_mode"`
	InnodbDataFilePath                string `toml:"innodb_data_file_path" json:"innodb_data_file_path"`
	InnodbDataHomeDir                 string `toml:"innodb_data_home_dir" json:"innodb_data_home_dir"`
	InnodbLogGroupHomeDir             string `toml:"innodb_log_group_home_dir" json:"innodb_log_group_home_dir"`
	InnodbBufferPoolSize              string `toml:"innodb_buffer_pool_size" json:"innodb_buffer_pool_size"`
	InnodbBufferPoolDumpAtShutdown    int    `toml:"innodb_buffer_pool_dump_at_shutdown" json:"innodb_buffer_pool_dump_at_shutdown"`
	InnodbBufferPoolDumpPct           int    `toml:"innodb_buffer_pool_dump_pct" json:"innodb_buffer_pool_dump_pct"`
	InnodbBufferPoolInstances         int    `toml:"innodb_buffer_pool_instances" json:"innodb_buffer_pool_instances"`
	InnodbBufferPoolLoadAtStartup     int    `toml:"innodb_buffer_pool_load_at_startup" json:"innodb_buffer_pool_load_at_startup"`
	InnodbFilePerTable                int    `toml:"innodb_file_per_table" json:"innodb_file_per_table"`
	InnodbFlushLogAtTrxCommit         int    `toml:"innodb_flush_log_at_trx_commit" json:"innodb_flush_log_at_trx_commit"`
	InnodbFlushMethod                 string `toml:"innodb_flush_method" json:"innodb_flush_method"`
	InnodbFlushNeighbors              int    `toml:"innodb_flush_neighbors" json:"innodb_flush_neighbors"`
	InnodbIOCapacity                  int    `toml:"innodb_io_capacity" json:"innodb_io_capacity"`
	InnodbIOCapacityMax               int    `toml:"innodb_io_capacity_max" json:"innodb_io_capacity_max"`
	InnodbLargePrefix                 int    `toml:"innodb_large_prefix" json:"innodb_large_prefix"`
	InnodbLockWaitTimeout             int    `toml:"innodb_lock_wait_timeout" json:"innodb_lock_wait_timeout"`
	InnodbLogBufferSize               string `toml:"innodb_log_buffer_size" json:"innodb_log_buffer_size"`
	InnodbLogFilesInGroup             int    `toml:"innodb_log_files_in_group" json:"innodb_log_files_in_group"`
	InnodbLogFileSize                 string `toml:"innodb_log_file_size" json:"innodb_log_file_size"`
	InnodbLruScanDepth                int    `toml:"innodb_lru_scan_depth" json:"innodb_lru_scan_depth"`
	InnodbMaxUndoLogSize              string `toml:"innodb_max_undo_log_size" json:"innodb_max_undo_log_size"`
	InnodbOnlineAlterLogMaxSize       string `toml:"innodb_online_alter_log_max_size" json:"innodb_online_alter_log_max_size"`
	InnodbOpenFiles                   int    `toml:"innodb_open_files" json:"innodb_open_files"`
	InnodbPageCleaners                int    `toml:"innodb_page_cleaners" json:"innodb_page_cleaners"`
	InnodbPageSize                    int    `toml:"innodb_page_size" json:"innodb_page_size"`
	InnodbPrintAllDeadlocks           int    `toml:"innodb_print_all_deadlocks" json:"innodb_print_all_deadlocks"`
	InnodbPurgeRsegTruncateFrequency  int    `toml:"innodb_purge_rseg_truncate_frequency" json:"innodb_purge_rseg_truncate_frequency"`
	InnodbPurgeThreads                int    `toml:"innodb_purge_threads" json:"innodb_purge_threads"`
	InnodbReadIOThreads               int    `toml:"innodb_read_io_threads" json:"innodb_read_io_threads"`
	InnodbSortBufferSize              string `toml:"innodb_sort_buffer_size" json:"innodb_sort_buffer_size"`
	InnodbStatsPersistentSamplePages  int    `toml:"innodb_stats_persistent_sample_pages" json:"innodb_stats_persistent_sample_pages"`
	InnodbStrictMode                  int    `toml:"innodb_strict_mode" json:"innodb_strict_mode"`
	InnodbThreadConcurrency           int    `toml:"innodb_thread_concurrency" json:"innodb_thread_concurrency"`
	InnodbUndoLogs                    int    `toml:"innodb_undo_logs" json:"innodb_undo_logs"`
	InnodbUndoLogTruncate             int    `toml:"innodb_undo_log_truncate" json:"innodb_undo_log_truncate"`
	InnodbUndoTableSpaces             int    `toml:"innodb_undo_tablespaces" json:"innodb_undo_tablespaces"`
	InnodbWriteIOThreads              int    `toml:"innodb_write_io_threads" json:"innodb_write_io_threads"`
}

func NewConfig() *AgentConfig {
	cfg := &AgentConfig{}

	cfg.FlagSet = flag.NewFlagSet(defaultName, flag.ContinueOnError)
	fs := cfg.FlagSet
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", defaultName)
		fs.PrintDefaults()
	}
	// misc related configuration.
	fs.StringVar(&cfg.configFile, "config", "", fmt.Sprintf("path to the %s configuration file", defaultName))
	return cfg
}

func (cfg *AgentConfig) Parse(arguments []string) error {
	// Parse first to get config file
	perr := cfg.FlagSet.Parse(arguments)
	switch perr {
	case nil:
	case flag.ErrHelp:
		os.Exit(0)
	default:
		os.Exit(2)
	}
	// Load config file if specified.
	if cfg.configFile != "" {
		cfg.configFromFile(cfg.configFile)
	}
	return nil
}

// configFromFile loads configuration from toml file.
func (cfg *AgentConfig) configFromFile(path string) {
	_, err := toml.DecodeFile(path, cfg)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(2)
	}
}

func CreateComposeConf(cfg *AgentConfig) error {
	//strReplPasswd := Decode(ReplPasswd)
	//strRootPasswd := Decode(RootPasswd)
	filename := "/etc/" + cfg.DockerConf.Port + "_docker-compose.yml"
	MysqlSocket := "/tmp/mysql_" + cfg.DockerConf.Port + ".sock"
	ContainerName := cfg.DockerConf.Port + "_" + cfg.DockerConf.InstanceName
	ImageVersion := "moiot/moha:" + cfg.DockerConf.Version
	volumes1 := cfg.DockerConf.DataDir + ContainerName + ":/var/lib/mysql:rw"
	volumes2 := "/etc/" + cfg.DockerConf.Port + "_config.toml:/agent/config.toml"
	volumes3 := cfg.DockerConf.DataDir + cfg.DockerConf.Port + "_agentlog:/agent/log:rw"
	volumes4 := "/etc/my_" + cfg.DockerConf.Port + ".cnf:/etc/my_" + cfg.DockerConf.Port + ".cnf"
	volumes5 := "/tmp:/tmp"
	volumes6 := cfg.DockerConf.DataDir + ":" + cfg.DockerConf.DataDir
	volumes7 := "/etc/localtime:/etc/localtime:ro"
	Volumes := []string{volumes1, volumes2, volumes3, volumes4, volumes5, volumes6, volumes7}
	composecfg := &CreateDockerCfg{
		ComposeVersion: "3.3",
	}
	composecfg.Services = make(map[string]ServicesName)
	composecfg.Services[ContainerName] = ServicesName{
		Image:         ImageVersion,
		NetworkMode:   "host",
		EntryPoint:    "/agent/supervise",
		Volumes:       Volumes,
		ContainerName: ContainerName,
		Environment: Environment{
			MySQLDatabase:   cfg.DockerConf.InstanceName,
			MysqlPasswd:     cfg.DockerConf.AgentPasswd,
			MysqlReplPasswd: cfg.DockerConf.MysqlReplPasswd,
			MysqlReplUser:   cfg.DockerConf.MysqlReplUser,
			MysqlRootPasswd: cfg.DockerConf.MysqlRootPasswd,
			MysqlUser:       cfg.DockerConf.AgentUser,
			MysqlSocket:     MysqlSocket,
		},
	}
	dockeryaml, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		panic(err)
	}
	composeinfo, err := yaml.Marshal(&composecfg)
	if err != nil {
		fmt.Println(err.Error())
		logger.Error(err.Error())
		return err
	}
	_, err = dockeryaml.Write([]byte(string(composeinfo)))
	if err != nil {
		fmt.Println(err.Error())
		logger.Error(err.Error())
		return err
	}
	dockeryaml.Close()
	return nil

}

func CreateAgentConf(cfg *AgentConfig) error {
	//strReplPasswd := Decode(ReplPasswd)
	DefaultsFile := "--defaults-file=/etc/my_" + cfg.AgentConf.Port + ".cnf"
	ForkProcessArgs := []string{"docker-entrypoint.sh", "mysqld", DefaultsFile}
	IntPort, err := strconv.Atoi(cfg.AgentConf.Port)
	if err != nil {
		logger.Error(err.Error())
	}
	ClusterName := cfg.AgentConf.Port + "_" + cfg.AgentConf.InstanceName
	DataDir := "/tmp/" + cfg.AgentConf.Port + "_" + cfg.AgentConf.InstanceName
	ListenAddr := "http://127.0.0.1:1" + cfg.AgentConf.Port
	EtcdRootPath := "/dbproxy/" + cfg.AgentConf.ProxyName + "/ks_cfg/nodes/"
	InternalServiceHost := cfg.AgentConf.HostAddress + ":" + cfg.AgentConf.Port
	agentfile := "/etc/" + cfg.AgentConf.Port + "_config.toml"
	agentcfg := &Config{
		ClusterName:           ClusterName,
		LeaderLeaseTTL:        30,
		ShutdownThreshold:     5,
		DataDir:               DataDir,
		ListenAddr:            ListenAddr,
		EtcdURLs:              cfg.AgentConf.EtcdURL,
		RegisterTTL:           10,
		EtcdRootPath:          EtcdRootPath,
		EtcdUsername:          cfg.AgentConf.EtcdUser,
		EtcdPassword:          cfg.AgentConf.EtcdPasswd,
		LogLevel:              "info",
		LogFile:               "/agent/log/mysql-agent.log",
		LogMaxSize:            30,
		LogMaxDays:            5,
		LogCompress:           true,
		InternalServiceHost:   InternalServiceHost,
		ExternalServiceHost:   InternalServiceHost,
		ForkProcessFile:       "/usr/local/bin/docker-entrypoint.sh",
		ForkProcessArgs:       ForkProcessArgs,
		ForkProcessWaitSecond: 60,
	}
	agentcfg.DBConfig = DBConfig{
		Host:                cfg.AgentConf.HostAddress,
		User:                cfg.AgentConf.AgentUser,
		Password:            cfg.AgentConf.AgentPasswd,
		Port:                IntPort,
		Timeout:             "1s",
		ReplicationUser:     cfg.AgentConf.MysqlReplUser,
		ReplicationPassword: cfg.AgentConf.MysqlReplPasswd,
		ReplicationNet:      "%",
	}
	agentconf, err := os.OpenFile(agentfile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		panic(err)
	}
	toml.NewEncoder(agentconf).Encode(&agentcfg)
	agentconf.Close()
	return nil

}

func CreateMySQLConf(cfg *AgentConfig) error {
	var ServerID string
	IntPort, err := strconv.Atoi(cfg.MysqlConf.Port)
	if err != nil {
		logger.Error(err.Error())
	}
	StrSplitHostAddress := strings.Split(cfg.MysqlConf.HostAddress, ".")
	if len(StrSplitHostAddress) != 4 {
		logger.Error("IP Adress error")
		os.Exit(2)
	}
	ConFilePre := "/etc/my_" + cfg.MysqlConf.Port + ".cnf"
	SQLMode := cfg.MysqlConf.SQLMode
	BindAddress := cfg.MysqlConf.HostAddress
	if StrSplitHostAddress[2] == "0" {
		ServerID = "1" + StrSplitHostAddress[3] + cfg.MysqlConf.Port
	} else {
		ServerID = StrSplitHostAddress[2] + StrSplitHostAddress[3] + cfg.MysqlConf.Port
	}
	IntServerID, err := strconv.Atoi(ServerID)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(2)
	}
	Socket := "/tmp/mysql_" + cfg.MysqlConf.Port + ".sock"
	PidFile := "mysql_" + cfg.MysqlConf.Port + ".pid"
	Datadir := cfg.MysqlConf.DataDir + cfg.MysqlConf.Port + "_" + cfg.MysqlConf.InstanceName + "/"
	BufferSize := cfg.MysqlConf.BufferPool
	mysqlcfg := &MySQLCfg{}
	mysqlcfg.MySQLPara = MySQLPara{
		User:                              "mysql",
		SQLMode:                           SQLMode,
		AutoCommit:                        1,
		InitConnect:                       "SET NAMES utf8mb4",
		CharacterSetServer:                "utf8mb4",
		CollationServer:                   "utf8mb4_unicode_ci",
		EventScheduler:                    1,
		MaxAllowedPacket:                  "128M",
		LowerCaseTableNames:               1,
		BindAddress:                       BindAddress,
		ReportHost:                        BindAddress,
		ServerID:                          IntServerID,
		Port:                              IntPort,
		Socket:                            Socket,
		PidFile:                           PidFile,
		DataDir:                           Datadir,
		SkipSymbolicLinks:                 1,
		ShowCompatibility56:               1,
		ExplicitDefaultsForTimestamp:      0,
		DefaultStorageEngine:              "InnoDB",
		TransactionIsolation:              "READ-COMMITTED",
		ReadOnly:                          1,
		SkipNameResolve:                   1,
		WaitTimeout:                       7200,
		InteractiveTimeout:                7200,
		LockWaitTimeout:                   1800,
		MaxConnectErrors:                  1000000,
		MaxConnections:                    10000,
		BinlogFormat:                      "row",
		BinlogGtidGimpleRecovery:          1,
		BinlogRowsQueryLogEvents:          1,
		ExpireLogsDays:                    7,
		BinlogRowImage:                    "full",
		MaxBinlogSize:                     "256M",
		BackLog:                           900,
		GeneralLogFile:                    "mysql.log",
		LogBin:                            "mysql-bin",
		LogError:                          "error.log",
		LogQueriesNotUsingIndexes:         0,
		LogSlaveUpdates:                   1,
		LogSlowAdminStatements:            1,
		LogSlowSlaveStatements:            1,
		LogThrottleQueriesNotUsingIndexes: 10,
		LogTimestamps:                     "system",
		LongQueryTime:                     1,
		MasterInfoRepository:              "TABLE",
		RelayLogInfoRepository:            "TABLE",
		RelayLogRecovery:                  1,
		RelayLog:                          "relay-log",
		SlowQueryLog:                      1,
		SlowQueryLogFile:                  "slow.log",
		SyncBinlog:                        1,
		SecureFilePriv:                    "/data1/mysql",
		JoinBufferSize:                    "128M",
		KeyBufferSize:                     "256M",
		MinExaminedRowLimit:               100,
		ReadBufferSize:                    "16M",
		ReadRndBufferSize:                 "32M",
		SortBufferSize:                    "32M",
		TableDefinitionCache:              4096,
		TableOpenCache:                    4096,
		TableOpenCacheInstances:           64,
		ThreadCacheSize:                   64,
		TmpTableSize:                      "64M",
		EnforceGtidConsistency:            1,
		GtidMode:                          "on",
		SkipSlaveStart:                    1,
		SlaveParallelType:                 "LOGICAL_CLOCK",
		SlaveParallelWorkers:              16,
		SlavePreserveCommitOrder:          1,
		SlaveRowsSearchAlgorithms:         "INDEX_SCAN,HASH_SCAN",
		SlaveTransactionRetries:           128,
		InnodbAutoincLockMode:             2,
		InnodbDataFilePath:                "ibdata1:256M:autoextend",
		InnodbDataHomeDir:                 Datadir,
		InnodbLogGroupHomeDir:             Datadir,
		InnodbBufferPoolSize:              BufferSize,
		InnodbBufferPoolDumpAtShutdown:    1,
		InnodbBufferPoolDumpPct:           40,
		InnodbBufferPoolInstances:         8,
		InnodbLruScanDepth:                4096,
		InnodbMaxUndoLogSize:              "256M",
		InnodbBufferPoolLoadAtStartup:     1,
		InnodbFilePerTable:                1,
		InnodbFlushLogAtTrxCommit:         1,
		InnodbFlushMethod:                 "O_DIRECT",
		InnodbFlushNeighbors:              0,
		InnodbUndoTableSpaces:             3,
		InnodbWriteIOThreads:              16,
		InnodbIOCapacity:                  10000,
		InnodbIOCapacityMax:               20000,
		InnodbLargePrefix:                 1,
		InnodbLockWaitTimeout:             10,
		InnodbLogBufferSize:               "16M",
		InnodbLogFilesInGroup:             2,
		InnodbLogFileSize:                 "256M",
		InnodbOnlineAlterLogMaxSize:       "4G",
		InnodbOpenFiles:                   10000,
		InnodbPageCleaners:                16,
		InnodbPageSize:                    16384,
		InnodbPrintAllDeadlocks:           1,
		InnodbPurgeRsegTruncateFrequency:  128,
		InnodbPurgeThreads:                4,
		InnodbReadIOThreads:               16,
		InnodbSortBufferSize:              "64M",
		InnodbStatsPersistentSamplePages:  64,
		InnodbStrictMode:                  1,
		InnodbThreadConcurrency:           64,
		InnodbUndoLogs:                    128,
		InnodbUndoLogTruncate:             1,
	}
	mysqlconf, err := os.OpenFile(ConFilePre, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		panic(err)
	}
	toml.NewEncoder(mysqlconf).Encode(&mysqlcfg)
	mysqlconf.Close()
	return nil
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

//简单加密解密算法
//func Encode(data string) string {
//	content := *(*[]byte)(unsafe.Pointer((*reflect.SliceHeader)(unsafe.Pointer(&data))))
//	coder := base64.NewEncoding(BASE64Table)
//	return coder.EncodeToString(content)
//}
//
//func Decode(data string) string {
//	coder := base64.NewEncoding(BASE64Table)
//	result, _ := coder.DecodeString(data)
//	return *(*string)(unsafe.Pointer(&result))
//}

func main() {
	logger.InitLogger("moha.log")
	cfg := NewConfig()
	if err := cfg.Parse(os.Args[1:]); err != nil {
		logger.Error(err.Error())
		os.Exit(2)
	}
	mysqlconf := "/etc/my_" + cfg.MysqlConf.Port + ".cnf"
	agentconf := "/etc/" + cfg.AgentConf.Port + "_config.toml"
	composeconf := "/etc/" + cfg.DockerConf.Port + "_docker-compose.yml"
	fileMySQL, _ := PathExists(mysqlconf)
	fikeAgent, _ := PathExists(agentconf)
	fileCompose, _ := PathExists(composeconf)
	if fileMySQL != false || fikeAgent != false || fileCompose != false {
		logger.Error("configure file existed")
		fmt.Println("configure file existed")
		os.Exit(2)
	}
	err := CreateMySQLConf(cfg)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(2)
	}
	err = CreateAgentConf(cfg)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(2)
	}
	err = CreateComposeConf(cfg)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(2)
	}
}
