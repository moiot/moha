// switch.go
//Date          28/06/2018
//Purpose       moha manual sitchover
// 1 获取用户名和密码,读取配置文件
// 2 获取所有主从IP和端口
// 3 检查当前正在运行连接数，当系统连接数大于100，中止此次计划内切换
//   检查一下主从是否都OK，如果从库有问题，则提示具体有问题从库，并中止此次计划内切换
//	 检查一下每个从库主从延迟，如果主从延迟大于10S，中止此次计划内切换
//   设置read_only;
//		如果成功：开始执行计划内切换；检查计划内切换是否成功
//		如果失败：重新将主库设置只读
// 关注目标：计划外切换失败，需要关注是否节点状态和之前一样；可通过日志查看到之前的节点信息

//change pos to gtid

package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"logger"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"flag"
	"github.com/BurntSushi/toml"
	"github.com/coreos/etcd/clientv3" //"encoding/json"
	gmysql "github.com/siddontang/go-mysql/mysql"
)

const (
	dialEtcdTimeout = time.Second * 5
	MySQLTimeout = time.Second * 1
)

var (
	h            bool
	instanceport string
)

func init() {
	flag.BoolVar(&h, "h", false, "this help")
	flag.StringVar(&instanceport, "instanceport", "3306", "instance port you want switch")
	flag.Usage = usage
}

func usage() {
	fmt.Fprintf(os.Stderr, `switch  version 2.0
    Usage: switch [h instanceport] [-instanceport=strMysqlPort]

  Options:
  `)
	flag.PrintDefaults()
}

// DBConfig is the MySQL connection configuration
type DBConfig struct {
	MysqlHost string `toml:"host" json:"host"`
	MysqlUser string `toml:"user" json:"user"`
	MysqlPwd  string `toml:"password" json:"password"`
	MysqlPort int    `toml:"port" json:"port"`
}

// Config is etcd connection configurationn
type Config struct {
	EtcdURLs     string   `toml:"etcd-urls" json:"etcd-urls"`
	EtcdRootPath string   `toml:"etcd-root-path" json:"etcd-root-path"`
	EtcdUsername string   `toml:"etcd-username" json:"etcd-username"`
	EtcdPassword string   `toml:"etcd-password" json:"etcd-password"`
	EtcdCluster  string   `toml:"cluster-name" json:"cluster-name"`
	EtcdHostPort string   `toml:"internal-service-host" json:"internal-service-host"`
	Db           DBConfig `toml:"db-config" json:"db-config"`
}

// --------------------------------MySQL---------------------------------------------

// CreateDB creates db connection using the cfg
func CreateDB(user string, password string, host string, port int) (*sql.DB, error) {
	dbDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8&interpolateParams=true&readTimeout=%s&writeTimeout=%s&timeout=%s",
		user, password, host, port, MySQLTimeout, MySQLTimeout, MySQLTimeout)
	db, err := sql.Open("mysql", dbDSN)
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

// GetSlaveStatus runs `show slave status` ans return the result as a map[string]string
func GetSlaveStatus(db *sql.DB) (map[string]string, error) {
	r, err := db.Query("SHOW SLAVE STATUS ")
	if err != nil {
		return nil, err
	}
	defer r.Close()
	columns, err := r.Columns()
	if err != nil {
		return nil, err
	}
	data := make([][]byte, len(columns))
	dataP := make([]interface{}, len(columns))
	for i := 0; i < len(columns); i++ {
		dataP[i] = &data[i]
	}
	if r.Next() {
		r.Scan(dataP...)
	}

	resultMap := make(map[string]string)
	for i, column := range columns {
		resultMap[column] = string(data[i])
	}
	return resultMap, nil
}

// threadsRunning runs `show global status like "Threads_running";` as  a map[string]string
func threadsRunning(masterdb *sql.DB) (int, error) {
	var masterRunningThread int
	var nullPtr interface{}
	rows, err := masterdb.Query("show global status like \"Threads_running\"")
	if err != nil {
		return 999, err
	}
	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&nullPtr, &masterRunningThread)
		if err != nil {
			return 9999, err
		}
	}
	return masterRunningThread, nil
}

type GTIDSet struct {
	*gmysql.MysqlGTIDSet
}

func masterStatus(masterdb *sql.DB) (string, int, string, error) {
	var (
		binlogGtid       string
		binlogFile string
		binlogPos        int
		nullPtr    interface{}
	)
	rows, err := masterdb.Query("show master status")
	if err != nil {
		return "nil", 9999, "nil",err
	}
	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&binlogFile, &binlogPos, &nullPtr, &nullPtr, &binlogGtid)
		if err != nil {
			return "nil", 9999, "nil",err
		}
	}
	return binlogFile, binlogPos, binlogGtid, nil
}

// GetServerUUID returns the uuid of current mysql server
func getServerUUID(db *sql.DB) (string, error) {
	var masterUUID string
	rows, err := db.Query(`SELECT @@server_uuid;`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&masterUUID)
		if err != nil {
			return "", err
		}
	}
	if rows.Err() != nil {
		return "", err
	}
	return masterUUID, nil
}

func parseGTIDSet(gtidStr string) (GTIDSet, error) {
	gs, err := gmysql.ParseMysqlGTIDSet(gtidStr)
	if err != nil {
		return GTIDSet{}, err
	}

	return GTIDSet{gs.(*gmysql.MysqlGTIDSet)}, nil
}


func getTxnIDFromGTIDStr(gtidStr, serverUUID string) (int64, error) {

	gtidSet, err := parseGTIDSet(gtidStr)
	if err != nil {
		return 0, err
	}
	uuidSet, ok := gtidSet.Sets[serverUUID]
	if !ok {
		return 0, err
	}
	intervalLen := len(uuidSet.Intervals)
	if intervalLen == 0 {
		return 0, err
	}
	// assume the gtidset is continuous, only pick the last one
	return uuidSet.Intervals[intervalLen-1].Stop, nil
}


//---------------------------------MySQL Agent API ----------------------------------
func getMysqlAgent(agentapi string) (result bool) {
	client := &http.Client{}
	reqest, err := http.NewRequest("GET", agentapi, nil)
	if err != nil {
		logger.Error("approved agenntapi has problem")
	}
	reqest.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	reqest.Header.Set("Accept-Charset", "GBK,utf-8;q=0.7,*;q=0.3")
	reqest.Header.Set("Accept-Encoding", "gzip,deflate,sdch")
	reqest.Header.Set("Accept-Language", "zh-CN,zh;q=0.8")
	reqest.Header.Set("Cache-Control", "max-age=0")
	//reqest.Header.Set("Connection", "keep-alive")
	reqest.Header.Set("Connection", "keep-alive")
	reqest.Header.Set("User-Agent", "chrome 100")
	response, _ := client.Do(reqest)
	if response.StatusCode == 200 {
		body, _ := ioutil.ReadAll(response.Body)
		bodystr := string(body)
		if strings.Contains(bodystr, "success") {
			logger.Info(bodystr)
			return true
		}
		logger.Error(bodystr)
		return false
	}
	body, _ := ioutil.ReadAll(response.Body)
	bodystr := string(body)
	logger.Error(bodystr)
	return false

}

//-----------------------------Get ETCD Info ----------------------------------------
func initEtcdClient(clusterUrls, etcdUser, etcdUserPass string) (client clientv3.KV, err error) {
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

//from etcd  get instancegroup master node info
func getInstanceGroupMaster(cfg *Config) (map[string]string, error) {
	masterInfo := make(map[string]string)
	client, err := initEtcdClient(cfg.EtcdURLs, cfg.EtcdUsername, cfg.EtcdPassword)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(-1)
	} else {
		resp, err := client.Get(context.Background(), cfg.EtcdRootPath+cfg.EtcdCluster+"/master")
		if err != nil {
			logger.Error(err.Error())
			os.Exit(-1)
		}
		if len(resp.Kvs) <= 0 {
			logger.Error("Instance group " + cfg.EtcdCluster + " Master Info is null")
			os.Exit(-1)
		}
		masterInfo["ip"] = strings.Split(string(resp.Kvs[0].Value), ":")[0]
		masterInfo["port"] = strings.Split(string(resp.Kvs[0].Value), ":")[1]
	}
	return masterInfo, nil
}

//from etcd  get instancegroup slave node info
func getInstanceGroupSlave(cfg *Config) ([]string, error) {
	slaveInfo := make([]string, 0, 3)
	client, err := initEtcdClient(cfg.EtcdURLs, cfg.EtcdUsername, cfg.EtcdPassword)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(-1)
	}
	resp, err := client.Get(context.Background(), cfg.EtcdRootPath+cfg.EtcdCluster+"/slave", clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		logger.Error(err.Error())
		os.Exit(-1)
	}
	if len(resp.Kvs) <= 0 {
		logger.Error("Instance group " + cfg.EtcdCluster + " Slave Info is null")
		logger.Info(resp.Kvs)
		os.Exit(-1)
	}
	for i := 0; i < len(resp.Kvs); i++ {
		if resp.Kvs[i].Key != nil {
			strings.Split(string(resp.Kvs[i].Key), "/")
			_, ipPort := path.Split(string(resp.Kvs[i].Key))
			slaveInfo = append(slaveInfo, ipPort)
		} else {
			logger.Info("Slave Key is nil")
		}
	}
	return slaveInfo, nil
}

//切换前预检查
func prefixSwitchCheck(cfg *Config, masterNode map[string]string, slaveNode []string) (bool, error) {
	//   检查当前正在运行连接数，当系统连接数大于100，中止此次计划内切换
	//   检查一下主从是否都OK，如果从库有问题，则提示具体有问题从库，并中止此次计划内切换
	//	 检查一下每个从库主从延迟，如果主从延迟大于10S，中止此次计划内切换
	masterport, _ := strconv.Atoi(masterNode["port"])
	masterdb, err := CreateDB(cfg.Db.MysqlUser, cfg.Db.MysqlPwd, masterNode["ip"], masterport)
	masterThreadsRuning, err := threadsRunning(masterdb)
	if err != nil {
		fmt.Println(err.Error())
		logger.Error(err.Error())
		return false, err
	}
	if masterThreadsRuning > 100 {
		fmt.Println(masterNode["ip"] + ":" + masterNode["port"] + " master running thread  greater than 100,plan switch exit")
		logger.Error(masterNode["ip"] + ":" + masterNode["port"] + " master running thread  greater than 100,plan switch exit")
		return false, nil
	} else {
		fmt.Println(masterNode["ip"] + ":" + masterNode["port"] + " master running thread is below 100,check continue")
		logger.Info(masterNode["ip"] + ":" + masterNode["port"] + " master running thread is below 100,check continue")
	}

	// get show master status info
	serverUUID,err := getServerUUID(masterdb)
	if err != nil {
		fmt.Println(err.Error())
		logger.Error(err.Error())
	}
	fmt.Println(serverUUID)
	_,_,mastergtid,err := masterStatus(masterdb)
	if err != nil {
		fmt.Println(err.Error())
		logger.Error(err.Error())
	}
	fmt.Println(mastergtid)

	for i := 0; i < len(slaveNode); i++ {
		var (
			retrievedMasterLastGtid int64
			executedLasteGtid int64
		)
		slaveIP := strings.Split(slaveNode[i], ":")[0]
		tslavePort := strings.Split(slaveNode[i], ":")[1]
		slavePort, _ := strconv.Atoi(tslavePort)
		slaveDBInfo, err := CreateDB(cfg.Db.MysqlUser, cfg.Db.MysqlPwd, slaveIP, slavePort)
		if err != nil {
			logger.Error(err)
		}
		slaveinfo, err := GetSlaveStatus(slaveDBInfo)
		if slaveinfo["Slave_IO_Running"] == "Yes" && slaveinfo["Slave_SQL_Running"] == "Yes" {
			fmt.Println(slaveNode[i] + " Slave_IO_Running and Slave_SQL_Running thread is ok,check continue")
			logger.Info(slaveNode[i] + " Slave_IO_Running and Slave_SQL_Running thread is ok,check continue")
		} else {
			fmt.Println(slaveNode[i] + " Slave_IO_Running: " + slaveinfo["Slave_IO_Running"] + " Slave_SQL_Running:" + slaveinfo["Slave_SQL_Running"] + " ,plan switch exit")
			logger.Error(slaveNode[i] + " Slave_IO_Running: " + slaveinfo["Slave_IO_Running"] + " Slave_SQL_Running:" + slaveinfo["Slave_SQL_Running"] + " ,plan switch exit")
			return false, nil
		}
		if slaveinfo["Seconds_Behind_Master"] <= "20" {
			fmt.Println(slaveNode[i] + " Seconds_Behind_Master is below 20s,check OK")
			logger.Info(slaveNode[i] + " Seconds_Behind_Master is below 20s,check OK")
		} else {
			fmt.Println(slaveNode[i] + " Seconds_Behind_Master is greater than 20s,plan switch exit")
			logger.Info(slaveNode[i] + " Seconds_Behind_Master is greater than 20s,plan switch exit")
			return false, nil
		}
		//todo 增加比较从库没有应用的事务数量比较 Retrieved_Gtid_Set == Executed_Gtid_Set

		retrievedMasterLastGtid,_ = getTxnIDFromGTIDStr(slaveinfo["Executed_Gtid_Set"],serverUUID)
		executedLasteGtid,_ = getTxnIDFromGTIDStr(slaveinfo["Executed_Gtid_Set"],serverUUID)
		gtidNotExec:=retrievedMasterLastGtid-executedLasteGtid
		strGtidNotExec := strconv.FormatInt(gtidNotExec,10)
		diffGtidEvent := fmt.Sprintf("%s this node has %s gitd event not exec",slaveIP,strGtidNotExec)
		fmt.Println(diffGtidEvent)

	}
	return true, nil
}

func checkConsistency(cfg *Config, masterNode map[string]string, slaveNode []string) (bool, error) {
	masterport, _ := strconv.Atoi(masterNode["port"])
	masterdb, err := CreateDB(cfg.Db.MysqlUser, cfg.Db.MysqlPwd, masterNode["ip"], masterport)
	binlogFileName, binlogPos, _, err := masterStatus(masterdb)

	strBinlogPos := strconv.Itoa(binlogPos)
	if err != nil {
		logger.Error(err)
		logger.Error("connect to master database fail")
		return false, err
	}
	for i := 0; i < len(slaveNode); i++ {
		slaveIP := strings.Split(slaveNode[i], ":")[0]
		tslavePort := strings.Split(slaveNode[i], ":")[1]
		slavePort, _ := strconv.Atoi(tslavePort)
		slavedb, err := CreateDB(cfg.Db.MysqlUser, cfg.Db.MysqlPwd, slaveIP, slavePort)
		if err != nil {
			logger.Error(err)
			return false, err
		}
		slaveinfo, err := GetSlaveStatus(slavedb)
		if err != nil {
			logger.Error(err)
			return false, err
		}
		if slaveinfo["Relay_Master_Log_File"] == binlogFileName && slaveinfo["Exec_Master_Log_Pos"] == strBinlogPos && slaveinfo["Seconds_Behind_Master"] == "0" {
			logger.Info(slaveNode[i] + " slave is approve master,check OK")
			fmt.Println(slaveNode[i] + " slave is approve master,check OK")
		} else {
			logger.Info(slaveNode[i] + " slave is not approve master,check fail")
			fmt.Println(slaveNode[i] + " slave is not approve master,check fail")
			return false, nil
		}
	}
	return true, nil
}

func main() {
	flag.Parse()
	if h {
		flag.Usage()
		os.Exit(-1)
	}
	filePath := "/etc/" + instanceport + "_config.toml"
	realSlaveInfo := make([]string, 0, 3)
	logger.InitLogger("mohaswitch.log")
	cfg := &Config{}
	_, err := toml.DecodeFile(filePath, cfg)
	if err != nil {
		fmt.Println(err.Error())
		logger.Error(err)
		os.Exit(-1)
	}
	masterNode, err := getInstanceGroupMaster(cfg)
	slaveNode, err := getInstanceGroupSlave(cfg)
	for i := 0; i < len(slaveNode); i++ {
		slaveIP := strings.Split(slaveNode[i], ":")[0]
		slavePort := strings.Split(slaveNode[i], ":")[1]
		if masterNode["ip"] == slaveIP && masterNode["port"] == slavePort {
			logger.Info(slaveNode[i] + " the slave is also master,not need register slave")
		} else {
			realSlaveInfo = append(realSlaveInfo, slaveNode[i])
			logger.Info(slaveNode[i] + " is slave ")
		}
	}
	checkResult, err := prefixSwitchCheck(cfg, masterNode, realSlaveInfo)
	if checkResult == true {
		setReadOnlyAPI := fmt.Sprintf(`http://%s:1%s/setReadOnly`, masterNode["ip"], masterNode["port"])
		setChangeMaster := fmt.Sprintf(`http://%s:1%s/changeMaster`, masterNode["ip"], masterNode["port"])
		setReadWrite := fmt.Sprintf(`http://%s:1%s//setReadWrite`, masterNode["ip"], masterNode["port"])
		getreadResult := getMysqlAgent(setReadOnlyAPI)
		if getreadResult == true {
			for i := 0; i < 10; i++ {
				getconsistency, err := checkConsistency(cfg, masterNode, realSlaveInfo)
				if err != nil {
					logger.Error("connect to database fail,recovery setreadonly and exit")
					fmt.Println("connect to database fail,recovery setreadonly and exit")
					//todo 进入recovery mode
					_ = getMysqlAgent(setReadWrite)
					os.Exit(-1)
				}
				if getconsistency == true {
					getchangeResult := getMysqlAgent(setChangeMaster)
					if getchangeResult == true {
						fmt.Println("change master success")
						logger.Info("change master success")
						os.Exit(-1)
					} else {
						//todo 回滚的时候，需要再次检查一下主从确实没有变化后，才将旧主库只读取消，防止主从都可写
						fmt.Println("change master fail,be careful")
						logger.Error("change master fail,be careful")
						readwrite := getMysqlAgent(setReadWrite)
						if readwrite == true {
							logger.Info("recovery readwrite success")
							fmt.Println("recovery readwrite success")
						} else {
							logger.Error("recovery readwrite fail")
							fmt.Println("recovery readwrite fail")
						}
					}
				}
				time.Sleep(1 * time.Second)
			}
			readwrite1 := getMysqlAgent(setReadWrite)
			if readwrite1 == true {
				fmt.Println("while deadline time,master slave not consistency，to set master readwrite success ")
				logger.Info("while deadline time,master slave not consistency，to set master readwrite success ")
			} else {
				fmt.Println("while deadline time,master slave not consistency，to set master readwrite fail,want dba to check ")
				logger.Error("while deadline time,master slave not consistency，to set master readwrite fail,want dba to check ")
			}
		}

	}
}
