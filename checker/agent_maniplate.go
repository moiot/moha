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

package checker

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"os/exec"

	"bytes"
	"io"

	"strings"

	"path"

	"git.mobike.io/database/mysql-agent/pkg/log"
	"github.com/juju/errors"
)

func (s *Server) runChangeMaster() error {
	// get leader
	leaderID, _, err := s.etcdClient.Get(s.ctx, leaderPath)
	if err != nil {
		return nil
	}

	parsedURL, err := url.Parse("http://" + string(leaderID))
	// hack logic
	u := parsedURL.Hostname()
	port := "1" + parsedURL.Port()

	_, err = http.Get(fmt.Sprintf("http://%s:%s/setReadOnly", u, port))
	time.Sleep(3 * time.Second)
	_, err = http.Get(fmt.Sprintf("http://%s:%s/changeMaster", u, port))
	return err
}

func (s *Server) deleteTerm(agentID string) error {
	return s.etcdClient.Delete(s.ctx, path.Join("election", "terms", agentID), false)
}

func (s *Server) getASlave() string {
	masterID, _ := s.getDBConnection()
	// hack ways
	var slaveID string
	for id := range s.cfg.IDContainerMapping {
		if id == masterID {
			continue
		}
		slaveID = id
		break
	}
	return slaveID
}

// RunStopSlave runs `stop slave` on the given DB
func RunStopSlave(rootConn *sql.DB) error {
	_, err := rootConn.Exec("STOP SLAVE;")
	return errors.Trace(err)
}

// RunStopAgent runs `docker stop <containerName>`
func RunStopAgent(containerName string) error {
	log.Info("run docker stop ", containerName)
	cmd := exec.Command("docker", "stop", containerName)
	return cmd.Run()
}

// RunStartAgent runs `docker start <containerName>`
func RunStartAgent(containerName string) error {
	log.Info("run docker start ", containerName)
	cmd := exec.Command("docker", "start", containerName)
	return cmd.Run()
}

// RunKill9Agent runs `docker exec <containerName> kill -9 <agentPID>`
func RunKill9Agent(containerName string) error {
	cmd := exec.Command("docker", "exec", containerName,
		"pgrep", "-f", "mysql-agent")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return errors.Errorf(fmt.Sprintf("err: %v ,stdout: %s, stderr: %s",
			err, out.String(), stderr.String()))
	}
	length := 0
	var pid string
	for length == 0 {
		bs, e := out.ReadBytes('\n')
		if length = len(bs); length == 0 {
			continue
		}
		pid = string(bs)
		if e != nil {
			if e == io.EOF {
				break
			}
			return errors.Trace(e)
		}
	}
	pid = strings.Trim(pid, "\n")
	log.Info("mysql agent pid in ", containerName, " is ", pid)
	cmd = exec.Command("docker", "exec", containerName,
		"kill", "-9", pid)
	var stderrBuf bytes.Buffer
	var stdoutBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	cmd.Stdout = &stdoutBuf
	err = cmd.Run()
	if err != nil {
		return errors.Errorf(fmt.Sprintf("err: %v ,stdout: %s, stderr: %s",
			err, stdoutBuf.String(), stderrBuf.String()))
	}
	return nil
}

// IPOf returns the ip of the container, given the container name/ID
func IPOf(node string) (string, error) {
	cmd := exec.Command("docker", "inspect", "-f",
		"'{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'",
		node)
	var stderrBuf bytes.Buffer
	var stdoutBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	cmd.Stdout = &stdoutBuf
	err := cmd.Run()
	if err != nil {
		return "", errors.Errorf(fmt.Sprintf("err: %v ,stdout: %s, stderr: %s",
			err, stdoutBuf.String(), stderrBuf.String()))
	}
	r := stdoutBuf.String()
	r = strings.Replace(r, "\n", "", -1)
	r = strings.Replace(r, "'", "", -1)
	return r, nil
}

// CheckCall runs the command, returned with the result and error
func CheckCall(command string) (result string, err error) {
	if command == "" {
		log.Error("command is empty")
		return "", errors.NotValidf("command is empty")
	}
	log.Info("going to run command: ", command)
	split := strings.Split(command, " ")
	splitCommand := make([]string, 0)
	for _, c := range split {
		if c == "" {
			continue
		}
		splitCommand = append(splitCommand, c)
	}
	log.Info("going to run split command: ", splitCommand)
	cmd := exec.Command(splitCommand[0], splitCommand[1:]...)
	var stderrBuf bytes.Buffer
	var stdoutBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	cmd.Stdout = &stdoutBuf
	err = cmd.Run()
	if err != nil {
		return "", errors.Errorf(fmt.Sprintf("err: %v ,stdout: %s, stderr: %s",
			err, stdoutBuf.String(), stderrBuf.String()))
	}
	result = stdoutBuf.String()
	return result, err
}

// PartitionOutgoing partitions the outgoing network for the partitionedAZ,
// with the partitionType
func PartitionOutgoing(partitionTemplate, partitionedAZ, partitionType string) error {
	if _, ok := azSet[partitionedAZ]; !ok {
		log.Errorf("the az %s is not in azSet %+v", partitionedAZ, azSet)
		return errors.Errorf("the az %s is not in azSet %+v", partitionedAZ, azSet)
	}
	query := partitionTemplate
	for ip, az := range ipAZMapping {
		if ip == "" || az == partitionedAZ {
			continue
		}
		query += fmt.Sprintf("--target %s ", ip)
	}
	partitionedNodes := azNodeMapping[partitionedAZ]
	for node := range partitionedNodes {
		nodeQuery := fmt.Sprintf("%s %s %s",
			query, partitionType, node)
		_, err := CheckCall(nodeQuery)
		if err != nil {
			log.Errorf("has error in executing %s, %+v",
				nodeQuery, err)
			return err
		}
	}
	return nil
}

// PartitionIncoming partitions the incoming network for the partitionedAZ,
// with the partitionType
func PartitionIncoming(partitionTemplate, partitionedAZ, partitionType string) error {
	if _, ok := azSet[partitionedAZ]; !ok {
		log.Errorf("the az %s is not in azSet %+v", partitionedAZ, azSet)
		return errors.Errorf("the az %s is not in azSet %+v", partitionedAZ, azSet)
	}
	query := partitionTemplate
	for ip := range azIPMapping[partitionedAZ] {
		query += fmt.Sprintf("--target %s ", ip)
	}
	for az, nodes := range azNodeMapping {
		if az == partitionedAZ {
			continue
		}
		for node := range nodes {
			nodeQuery := fmt.Sprintf("%s %s %s",
				query, partitionType, node)
			_, err := CheckCall(nodeQuery)
			if err != nil {
				log.Errorf("has error in executing %s, %+v",
					nodeQuery, err)
				return err
			}
		}
	}
	return nil
}
