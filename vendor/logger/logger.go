package logger

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	INFO = iota << 0
	DEBUG
	ERROR
	FATAL
)

const (
	defaultLogTimeFormat = "2006/01/02 15:04:05.000"
	defaultLogLevel      = INFO
	bufferMaxLen         = 1024 * 4
	tableName            = "manager_log"
)

var (
	levelStr = []string{"[INFO]", "[DEBUG]", "[ERROR]", "[FATAL]"}

	// global log instance
	globalLoggerInstance LoggerInter
)

type timeInterface interface {
	GetNow() string
	SetFromat(string)
}

type timeProvider struct {
	format string
}

func (t *timeProvider) GetNow() string {
	return time.Now().Format(t.format)
}

func (t *timeProvider) SetFormat(f string) {
	t.format = f
}

// For
type LoggerInter interface {
	WriteLog(...interface{})
	WriteLLog(int, ...interface{})
	SetLogLevel(int)
	SetLogFileName(string)
	Flush()
}

type logger struct {
	sync.Mutex
	buffer        bytes.Buffer
	level         int64
	bufferCurrLen uint64
	file          string
	mysqlUser     string
	mysqlPass     string
	mysqlAddr     string
	dbName        string
	db            *sql.DB
	fd            *os.File
	mysqlChan     chan string

	timeInterface *timeProvider
}

// TODO future support
func (l *logger) Flush() {
	l.fd.Close()
	l.db.Close()
	close(l.mysqlChan)
}

func (l *logger) GetLogLevel() string {
	return levelStr[l.level]
}

func (l *logger) GetLLogLevel(lv int) string {
	if lv > FATAL {
		return levelStr[FATAL]
	} else if lv < 0 {
		return levelStr[INFO]
	}
	return levelStr[lv]
}

func (l *logger) SetLogLevel(lv int) {
	atomic.StoreInt64(&l.level, int64(lv))
}

func (l *logger) SetLogFileName(f string) {
	l.file = f
}

func (l *logger) makeLogContent(s ...interface{}) {
	for _, c := range s {
		switch t := c.(type) {
		case int:
			l.buffer.WriteString(fmt.Sprintf(" %s ", t))
		case uint64:
			l.buffer.WriteString(fmt.Sprintf(" %s ", t))
		case float64:
			l.buffer.WriteString(fmt.Sprintf(" %f ", t))
		case string:
			l.buffer.WriteString(fmt.Sprintf(" %s ", t))
		default:
			l.buffer.WriteString(fmt.Sprintf(" %v ", t))
		}
	}
}

// log format : timestamp loglevel opername operator content
func (l *logger) WriteLog(s ...interface{}) {
	l.Lock()
	defer l.Unlock()
	l.buffer.WriteString(fmt.Sprintf("%s %s", l.timeInterface.GetNow(), l.GetLogLevel()))
	l.makeLogContent(s...)
	//l.mysqlChan <- l.buffer.String()
	l.buffer.WriteString("\n")
	l.fd.Write(l.buffer.Bytes())
	l.buffer.Reset()
}

// write Level Log
func (l *logger) WriteLLog(lv int, s ...interface{}) {
	l.Lock()
	defer l.Unlock()
	l.buffer.WriteString(fmt.Sprintf("%s %s", l.timeInterface.GetNow(), l.GetLLogLevel(lv)))
	l.makeLogContent(s...)
	//l.mysqlChan <- l.buffer.String()
	l.buffer.WriteString("\n")
	l.fd.Write(l.buffer.Bytes())
	l.buffer.Reset()
}

// TODO Rotate
func (l *logger) rotate() {

}

func (l *logger) writeToMysql(data *string) {
	var err error
	realSen := strings.Replace(*data, "\"", "'", -1)
	sqlSen := string("insert into ") + tableName + "(content) values(\"" + realSen + "\");"
	_, err = l.db.Exec(sqlSen)
	//_, err = l.db.Exec(sqlSen)
	if err != nil {
		l.WriteLLog(ERROR, "write to mysql error:", err.Error())
		return
	}
}

func (l *logger) writeToMysqlRoutine() {
	go func() {
		for {
			select {
			case data := <-l.mysqlChan:
				l.writeToMysql(&data)
			}
		}
	}()
}

func (l *logger) connectMysql() error {
	var err error
	l.db, err = sql.Open("mysql", l.mysqlUser+":"+l.mysqlPass+"@tcp("+l.mysqlAddr+")/"+l.dbName)
	if err != nil {
		return err
	}
	return nil
}

func (l *logger) OpenLogFile() error {
	var err error
	l.fd, err = os.OpenFile(l.file, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	return nil
}

func Debug(s ...interface{}) {
	globalLoggerInstance.WriteLLog(ERROR, s...)
}

func Error(s ...interface{}) {
	globalLoggerInstance.WriteLLog(ERROR, s...)
}

func Info(s ...interface{}) {
	globalLoggerInstance.WriteLog(s...)
}

func Fatal(s ...interface{}) {
	globalLoggerInstance.WriteLLog(FATAL, s...)
}

func InitLogger(filename string) error {
	l := new(logger)
	//l.dbName = dbName
	l.file = filename
	//l.mysqlUser = mUser
	//l.mysqlPass = mPass
	l.bufferCurrLen = bufferMaxLen
	l.level = defaultLogLevel
	//l.mysqlAddr = mAddr
	l.timeInterface = &timeProvider{}
	l.timeInterface.SetFormat(defaultLogTimeFormat)
	//l.mysqlChan = make(chan string)

	l.buffer.Grow(bufferMaxLen)

	if err := l.OpenLogFile(); err != nil {
		return err
	}


	//l.WriteToMysqlRoutine()
	globalLoggerInstance = l
	return nil
}
