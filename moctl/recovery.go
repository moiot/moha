package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/BurntSushi/toml"
	"github.com/coreos/etcd/clientv3"

	//"io/ioutil"
	"database/sql"
	"encoding/json"

	_ "github.com/go-sql-driver/mysql"

	//etr "github.com/juju/errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	gmysql "github.com/siddontang/go-mysql/mysql"
)

var (
	h               bool
	instanceport    string
	binlogbackupdir string
)

func init() {
	flag.BoolVar(&h, "h", false, "this help")
	flag.StringVar(&instanceport, "instanceport", "3000", "instance port you want recovery")
	flag.StringVar(&binlogbackupdir, "binlogbackupdir", "/data/backup/binlog", "diff binlog backup")
	flag.Usage = usage
}

func usage() {
	fmt.Fprintf(os.Stderr, `recovery  version 2.0
  Usage: switch [h instanceport] [-instanceport=strMysqlPort]

  Options:
  `)
	flag.PrintDefaults()
}

const (
	dialEtcdTimeout     = time.Second * 5
	recoveryComposeFile = "/etc/recoverycomposefile.yml"
)

//mysql show  master status
type MysqlCurrentStat struct {
	BinlogFile string `json:"File"`
	BinlogPos  string `json:"Pos"`
	BinlogGtid string `json:"GTID"`
	UUID       string `json:"UUID"`
}

// mysql connect info
type DBConfig struct {
	MysqlHost string `toml:"host" json:"host"`
	MysqlUser string `toml:"user" json:"user"`
	MysqlPwd  string `toml:"password" json:"password"`
	MysqlPort int    `toml:"port" json:"port"`
}

// agent config info
type Config struct {
	EtcdURLs     string   `toml:"etcd-urls" json:"etcd-urls"`
	EtcdRootPath string   `toml:"etcd-root-path" json:"etcd-root-path"`
	EtcdUsername string   `toml:"etcd-username" json:"etcd-username"`
	EtcdPassword string   `toml:"etcd-password" json:"etcd-password"`
	EtcdCluster  string   `toml:"cluster-name" json:"cluster-name"`
	EtcdHostPort string   `toml:"internal-service-host" json:"internal-service-host"`
	Db           DBConfig `toml:"db-config" json:"db-config"`
}

//padder main config
type WriteConfig struct {
	PadderConfig PadderConfig `toml:"padder" json:"padder"`
}

//padder setting config
type PadderConfig struct {
	BinLogList   []string     `toml:"binlog-list" json:"binlog-list"`
	EnableDelete bool         `toml:"enable-delete" json:"enable-delete"`
	MySQLConfig  *MySQLConfig `toml:"mysql" json:"mysql"`
}

//padder dest  main mysql config
type MySQLConfig struct {
	Target        *DBConfigNewMaster   `toml:"target" json:"target"`
	StartPosition *MySQLBinlogPosition `toml:"start-position" json:"start-position"`
}

// padder dest mysql config
type DBConfigNewMaster struct {
	Host     string `toml:"host" json:"host"`
	Location string `toml:"location" json:"location"`
	Username string `toml:"username" json:"username"`
	Password string `toml:"password" json:"password"`
	Port     int    `toml:"port" json:"port"`
	Schema   string `toml:"schema" json:"schema"`
}

//padder dest mysql binlog config
type MySQLBinlogPosition struct {
	BinLogFileName string `toml:"binlog-name" json:"binlog-name"`
	BinLogFilePos  uint32 `toml:"binlog-pos" json:"binlog-pos"`
}

// GTIDSet wraps mysql.MysqlGTIDSet
type GTIDSet struct {
	*gmysql.MysqlGTIDSet
}

func parseGTIDSet(gtidStr string) (GTIDSet, error) {
	gs, err := gmysql.ParseMysqlGTIDSet(gtidStr)
	if err != nil {
		return GTIDSet{}, err
	}

	return GTIDSet{gs.(*gmysql.MysqlGTIDSet)}, nil
}

func dbSet(db *sql.DB, setString string) error {
	_, err := db.Exec(setString)
	return err
}

func InitEtcdClient(clusterUrls, etcdUser, etcdUserPass string) (client clientv3.KV, err error) {
	cfg := clientv3.Config{
		Endpoints:   strings.Split(clusterUrls, ","),
		DialTimeout: dialEtcdTimeout,
		Username:    etcdUser,
		Password:    etcdUserPass,
	}

	cli, err := clientv3.New(cfg)
	if err != nil {
		return nil, err
	}
	return clientv3.NewKV(cli), nil
}

func GetEtcdSwitchInfo(cfg *Config, filePath string) (map[string]string, error) {
	mp := make(map[string]string)
	client, err := InitEtcdClient(cfg.EtcdURLs, cfg.EtcdUsername, cfg.EtcdPassword)
	if err != nil {
		fmt.Println("main", "main", "Init etcd client failed", err.Error())
		os.Exit(-1)
	}
	resp, err := client.Get(context.Background(), cfg.EtcdRootPath+cfg.EtcdCluster+"/switch/",
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortDescend),
		clientv3.WithLimit(1))
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(-1)
	}
	if len(resp.Kvs) <= 0 {
		fmt.Println("less than 0")
		os.Exit(-1)
	}
	var currentstat MysqlCurrentStat
	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &currentstat)
	fmt.Println(err)
	MohaConInfo := strings.Split(string(resp.Kvs[0].Key), "/")
	MohaSwitchIPPort := MohaConInfo[len(MohaConInfo)-1]
	if MohaSwitchIPPort == "" {
		return mp, errors.New("MohaSwitchIPPort is null")
	}
	if MohaSwitchIPPort == cfg.EtcdHostPort {
		return mp, errors.New("current node is not switch node,please check")
	}
	MohaPosInfo := strings.Split(string(resp.Kvs[0].Value), ",\"")
	if len(MohaPosInfo) <= 0 {
		fmt.Println("etcd switch postion info is null")
	}
	fmt.Println(string(resp.Kvs[0].Value))
	mp["MohaSwitchFile"] = currentstat.BinlogFile
	mp["MohaSwitchPost"] = currentstat.BinlogPos
	mp["MohaSwitchGtid"] = currentstat.BinlogGtid
	mp["MohaNewMasterIp"] = strings.Split(MohaSwitchIPPort, ":")[0]
	mp["MohaNewMasterPort"] = strings.Split(MohaSwitchIPPort, ":")[1]
	return mp, nil
}

func CreateRecoveryDockerConf(sourceComposeFile string, recoveryComposeFile string, mysqlport string) error {
	if _, err := os.Stat(recoveryComposeFile); err == nil {
		fmt.Printf("old recoveryComposeFile found, now remove it\n")
		os.Remove(recoveryComposeFile)
	}
	recoveryOpenFile, err := os.Create(recoveryComposeFile)
	if err != nil {
		return errors.New("create recoveryComposeFile file fail")
	}
	if _, err := os.Stat(sourceComposeFile); err != nil {
		return errors.New("sourceComposeFile  not Existed")
	}
	sourceOpenFile, err := os.Open(sourceComposeFile)
	if err != nil {
		return errors.New("sourceComposeFile  Open fail")
	}
	defer sourceOpenFile.Close()
	sourceBufferRead := bufio.NewReader(sourceOpenFile)
	for {
		a, _, c := sourceBufferRead.ReadLine()
		if c == io.EOF {
			break
		} else if strings.Contains(string(a), "entrypoint") {
			var strMysqlStart string
			strMysqlStart = "mysqld --defaults-file=/etc/my_" + mysqlport + ".cnf\n"
			strEntryPoint := strings.Replace(string(a), "/agent/supervise", strMysqlStart, 1)
			//fmt.Println(strEntryPoint)
			_, err := io.WriteString(recoveryOpenFile, strEntryPoint)
			if err != nil {
				return errors.New("write New recovery file fail")
			}
		} else if strings.Contains(string(a), "agentlog") {
			//fmt.Println(string(a) + ",this string removed")
			continue
		} else if strings.Contains(string(a), "container_name") {
			_, err := io.WriteString(recoveryOpenFile, string(a)+"_recovery\n")
			if err != nil {
				return errors.New("write New recovery file fail")
			}
		} else {
			_, err := io.WriteString(recoveryOpenFile, string(a)+"\n")
			if err != nil {
				return errors.New("write New recovery file fail")
			}
		}
	}
	return nil

}

func linuxSystemCommand(systemcommand string) (string, error) {
	systemcommandPipe, err := exec.Command("/bin/bash", "-c", systemcommand).Output()
	//fmt.Println(time.Now())
	if err != nil {
		return "null", errors.New(err.Error())
	}
	return string(systemcommandPipe), nil

}

// CreateDB creates db connection using the cfg
func CreateDB(cfg DBConfig) (*sql.DB, error) {
	dbDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8&interpolateParams=true",
		cfg.MysqlUser, cfg.MysqlPwd, cfg.MysqlHost, cfg.MysqlPort)
	db, err := sql.Open("mysql", dbDSN)
	//fmt.Println(dbDSN)
	if err != nil {
		return nil, err
	}
	return db, nil
}

//new master
// CreateDB creates db connection using the cfg
func createNewMasterDB(user, passwd, host string, port int) (*sql.DB, error) {
	dbDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8&interpolateParams=true", user, passwd, host, port)
	db, err := sql.Open("mysql", dbDSN)
	//fmt.Println(dbDSN)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// CloseDB closes the db connection
func CloseDB(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}

func getServerDataDir(db *sql.DB) (string, error) {
	var dataDir string
	rows, err := db.Query(`SELECT @@datadir;`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&dataDir)
		if err != nil {
			return "", err
		}
	}
	if rows.Err() != nil {
		return "", err
	}
	return dataDir, nil
}

// GetMasterStatus shows master status of MySQL.
func GetMasterStatus(db *sql.DB) (gmysql.Position, GTIDSet, error) {
	var (
		binlogPos gmysql.Position
		gs        GTIDSet
	)
	rows, err := db.Query(`SHOW MASTER STATUS`)
	if err != nil {
		return binlogPos, gs, err
	}
	defer rows.Close()

	rowColumns, err := rows.Columns()
	if err != nil {
		return binlogPos, gs, err
	}
	var (
		gtid       string
		binlogName string
		pos        uint32
		nullPtr    interface{}
	)
	for rows.Next() {
		if len(rowColumns) == 5 {
			err = rows.Scan(&binlogName, &pos, &nullPtr, &nullPtr, &gtid)
		} else {
			err = rows.Scan(&binlogName, &pos, &nullPtr, &nullPtr)
		}
		if err != nil {
			return binlogPos, gs, err
		}

		binlogPos = gmysql.Position{
			Name: binlogName,
			Pos:  pos,
		}

		gs, err = parseGTIDSet(gtid)
		if err != nil {
			return binlogPos, gs, err
		}
	}
	if rows.Err() != nil {
		return binlogPos, gs, rows.Err()
	}

	return binlogPos, gs, nil
}

//获取binlog list
func getBinlogList(cfg *Config, strBinlogFile string) ([]string, error) {
	//连接mysql
	binlogSlice := make([]string, 0, 20)
	binlogStrStr := strings.Split(string(strBinlogFile), ".")[1]
	binlogPreName := strings.Split(string(strBinlogFile), ".")[0]
	fmt.Println(binlogStrStr, binlogPreName)
	binlogStrNum, err := strconv.ParseInt(string(binlogStrStr), 10, 64)
	if err != nil {
		fmt.Println(err.Error())
		return binlogSlice, errors.New(err.Error())
	}
	fmt.Println("debug:", strBinlogFile, binlogStrNum)
	db, err := CreateDB(cfg.Db)
	if err != nil {
		fmt.Println("get mysql conn fail")
	}
	var (
		pos gmysql.Position
		//gtidSet GTIDSet
	)
	for i := 0; i <= 10; i++ {
		pos, _, err = GetMasterStatus(db)
		if err == nil {
			break
		}
		time.Sleep(time.Duration(1) * time.Second)
		if i == 10 {
			fmt.Println("in deadline time, old master start  ")
			return nil, err
		}
	}
	oldMasterLastBinlogFile := pos.Name
	binlogStpStr := strings.Split(strings.Split(string(oldMasterLastBinlogFile), ".")[1], "\"")[0]
	binlogStpNum, err := strconv.ParseInt(string(binlogStpStr), 10, 64)
	if err != nil {
		fmt.Println(err.Error())
		return binlogSlice, errors.New(err.Error())
	}
	fmt.Println(strconv.FormatInt(binlogStrNum, 10))
	for i := binlogStrNum; i <= binlogStpNum; i++ {
		iStr := strconv.FormatInt(i, 10)
		iStrLen := strings.Count(iStr, "") - 1
		if iStrLen == 1 {
			binlogFileNameLenTmp := binlogPreName + ".00000" + iStr
			binlogSlice = append(binlogSlice, binlogFileNameLenTmp)
		} else if iStrLen == 2 {
			binlogFileNameLenTmp := binlogPreName + ".0000" + iStr
			binlogSlice = append(binlogSlice, binlogFileNameLenTmp)
		} else if iStrLen == 3 {
			binlogFileNameLenTmp := binlogPreName + ".000" + iStr
			binlogSlice = append(binlogSlice, binlogFileNameLenTmp)
		} else if iStrLen == 4 {
			binlogFileNameLenTmp := binlogPreName + ".00" + iStr
			binlogSlice = append(binlogSlice, binlogFileNameLenTmp)
		} else if iStrLen == 5 {
			binlogFileNameLenTmp := binlogPreName + ".0" + iStr
			binlogSlice = append(binlogSlice, binlogFileNameLenTmp)
		} else if iStrLen == 6 {
			binlogFileNameLenTmp := binlogPreName + "." + iStr
			binlogSlice = append(binlogSlice, binlogFileNameLenTmp)
		} else {
			fmt.Println("binlog list error,please check")
		}
	}
	return binlogSlice, nil

}

func copyFile(source, dest string) bool {
	if source == "" || dest == "" {
		fmt.Println("source or dest is null")
		return false
	}
	sourceOpen, err := os.Open(source)
	if err != nil {
		fmt.Println(err.Error())
		return false
	}
	defer sourceOpen.Close()
	destOpen, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 644)
	if err != nil {
		fmt.Println(err.Error())
		return false
	}
	defer destOpen.Close()
	_, copyErr := io.Copy(destOpen, sourceOpen)
	if copyErr != nil {
		fmt.Println(copyErr.Error())
		return false
	} else {
		return true
	}
}

func main() {

	flag.Parse()
	if h {
		flag.Usage()
		os.Exit(-1)
	}
	intMysqlPort, err := strconv.Atoi(instanceport)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(-1)
	}
	filePath := "/etc/" + instanceport + "_config.toml"
	sourceComposeFile := "/etc/" + instanceport + "_docker-compose.yml"
	//配置文件参数初始化
	cfg := &Config{}
	_, err = toml.DecodeFile(filePath, cfg)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(-1)
	}

	writecfg := &WriteConfig{}
	//判断生产环境docker容器是否处于运行状态，如果处于运行状态，则停止所有动作
	strDockerRun := "docker ps | grep " + cfg.EtcdCluster + "|wc -l"
	productDocker, err := linuxSystemCommand(strDockerRun)
	if err != nil {
		fmt.Println(err.Error())
	}
	if strings.Replace(productDocker, "\n", "", -1) == "1" {
		fmt.Println("product docker images runing,script exit")
		os.Exit(-1)
	}

	//通过配置文件获取etcd的连接信息，通过etcd获取切换后的新主切换时，新主同步到旧主的点位，GTID信息
	mp, err := GetEtcdSwitchInfo(cfg, filePath)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(-1)
	}
	//mp["MohaNewMasterIp"]
	newMasterInfo, err := createNewMasterDB(cfg.Db.MysqlUser, cfg.Db.MysqlPwd, mp["MohaNewMasterIp"], intMysqlPort)
	mysqlDataDir, err := getServerDataDir(newMasterInfo)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(-1)
	}

	//通过生产使用的docker compose file，生成做flashback需要使用的compose文件，降低出错几率
	err = CreateRecoveryDockerConf(sourceComposeFile, recoveryComposeFile, instanceport)
	if err != nil {
		fmt.Println(err.Error())
	}
	//启动恢复需要的docker
	runRecoveryDocker := "docker-compose -f " + recoveryComposeFile + " up -d"
	_, err = linuxSystemCommand(runRecoveryDocker)
	if err != nil {
		fmt.Println(err.Error())
	}
	//得到需要恢复的binlog slice
	fmt.Println(mp["MohaSwitchFile"])
	binlogRecoverSlice, err := getBinlogList(cfg, mp["MohaSwitchFile"])
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(-1)
	}
	//备份新主库未同步完的binlog
	mkbinlogdir := "mkdir -p " + binlogbackupdir
	_, err = linuxSystemCommand(mkbinlogdir)
	if err != nil {
		fmt.Println(err.Error())
	}
	recoveryBinlogList := make([]string, 0, 20)
	for _, i := range binlogRecoverSlice {
		sourcefile := mysqlDataDir + i
		destfile := binlogbackupdir + i
		recoveryBinlogList = append(recoveryBinlogList, destfile)
		isSuccessCopy := copyFile(sourcefile, destfile)
		if isSuccessCopy == false {
			fmt.Println("binlog file backup file")
			os.Exit(-1)
		}
	}
	//falshback binlog.output
	binlogFlashbackSlice := make([]string, 0, 20)
	for j, i := range binlogRecoverSlice {
		sourcefile := mysqlDataDir + i
		destfile := binlogbackupdir + i
		if j == 0 {
			flashbackCmd := "flashback --binlogFileNames=" + sourcefile + " --start-position=" + mp["MohaSwitchPost"] + " --outBinlogFileNameBase=" + destfile
			binlogFlashbackSlice = append(binlogFlashbackSlice, destfile+".flashback")
			_, err := linuxSystemCommand(flashbackCmd)
			if err != nil {
				fmt.Println(err.Error())
			}
		} else {
			flashbackCmd := "flashback --binlogFileNames=" + sourcefile + " --outBinlogFileNameBase=" + destfile
			binlogFlashbackSlice = append(binlogFlashbackSlice, destfile+".falshback")
			_, err := linuxSystemCommand(flashbackCmd)
			if err != nil {
				fmt.Println(err.Error())
			}
		}
	}

	for _, i := range binlogFlashbackSlice {
		flashbackMysql := "mysqlbinlog --skip-gtids " + i + " | mysql -u " + cfg.Db.MysqlUser + " -p" + cfg.Db.MysqlPwd + " -h" + cfg.Db.MysqlHost + " -P" + instanceport
		_, err := linuxSystemCommand(flashbackMysql)
		if err != nil {
			fmt.Println(err.Error())
		}
	}
	db, err := CreateDB(cfg.Db)
	resetMaster := "reset master"
	resetGtid := "SET @@GLOBAL.GTID_PURGED=\"" + mp["MohaSwitchGtid"] + "\""
	fmt.Println(resetGtid)
	err = dbSet(db, resetMaster)
	if err != nil {
		fmt.Println(err.Error())
	}
	err = dbSet(db, resetGtid)
	if err != nil {
		fmt.Println(err.Error())
	}
	writecfg.PadderConfig = PadderConfig{
		BinLogList:   recoveryBinlogList,
		EnableDelete: true,
	}
	MohaNewMasterPortNum, _ := strconv.Atoi(mp["MohaNewMasterPort"])
	writecfg.PadderConfig.MySQLConfig = &MySQLConfig{}
	writecfg.PadderConfig.MySQLConfig.Target = &DBConfigNewMaster{
		Location: "Asia/Shanghai",
		Schema:   "test",
		Host:     mp["MohaNewMasterIp"],
		Port:     MohaNewMasterPortNum,
		Username: cfg.Db.MysqlUser,
		Password: cfg.Db.MysqlPwd,
	}
	MohaBinlogFilePos, _ := strconv.Atoi(mp["MohaSwitchPost"])
	writecfg.PadderConfig.MySQLConfig.StartPosition = &MySQLBinlogPosition{
		BinLogFileName: string(mp["MohaSwitchFile"]),
		BinLogFilePos:  uint32(MohaBinlogFilePos),
	}
	paddercfg, err := os.OpenFile("padder.toml", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		panic(err)
	}
	toml.NewEncoder(paddercfg).Encode(&writecfg)
	paddercfg.Close()
	//manual operator
	fmt.Println("1、fist check flashback success or not\n 2、restart docker by product docker compose file\n 3、Modify padder.toml to compensate diff data")
}
