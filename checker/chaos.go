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
	"math/rand"
	"os"
	"path"
	"sync/atomic"
	"time"

	"github.com/moiot/moha/pkg/log"
	"github.com/juju/errors"
)

// ChaosRegistry registers chaos situation
var ChaosRegistry map[string]func(s *Server) error

func init() {
	ChaosRegistry = make(map[string]func(s *Server) error)
	ChaosRegistry["change_master"] = changeMasterCheck
	ChaosRegistry["stop_master_agent"] = stopMasterAgentCheck
	ChaosRegistry["stop_slave_agent"] = stopSlaveAgentCheck
	ChaosRegistry["partition_one_az"] = partitionOneAZCheck
	ChaosRegistry["partition_master_az"] = partitionMasterAZCheck
	ChaosRegistry["spm"] = singlePointMasterCheck
}

func changeMasterCheck(s *Server) error {

	// 1. check changeMaster works fine
	err := s.runChangeMaster()
	if err != nil {
		log.Error("runChangeMaster has error ", err)
		return errors.Trace(err)
	}
	// wait for the whole change master process done
	time.Sleep(10 * time.Second)

	// 2. stop one slave SQL Thread, check another slave becomes the new master after changing master
	masterID, _ := s.getDBConnection()
	slaveID := s.getASlave()
	rootConn, err := s.buildDBRootConnFromID(slaveID)
	RunStopSlave(rootConn)
	time.Sleep(500 * time.Millisecond)
	s.runChangeMaster()

	newMasterID := masterID
	for newMasterID == masterID || newMasterID == "" {
		newMasterID, _ = s.getDBConnection()
		if newMasterID != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if slaveID == newMasterID {
		log.Error("not the latest slave becomes the new Master")
		os.Exit(1)
	}

	atomic.StoreInt32(&s.chaosJobDone, 1)
	return nil

}

func stopMasterAgentCheck(s *Server) error {
	// 3. shutdown master, check another slave becomes master
	masterID, _ := s.getDBConnection()
	masterNode := s.getContainerFromID(masterID)
	RunStopAgent(masterNode)
	stopCh := make(chan interface{})
	timer := time.NewTimer(60 * time.Second)
	var newMasterID string
	go func() {
		for newMasterID == masterID {
			newMasterID, _ = s.getDBConnection()
			if newMasterID != masterID {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		stopCh <- "done"
	}()
	select {
	case <-stopCh:
		timer.Stop()
		break
	case <-timer.C:
		log.Error("timeout to wait for new master after stopping old master")
		os.Exit(1)
	}

	// RunStartAgent(masterNode)
	// sleep 5 second for data catchup
	time.Sleep(5 * time.Second)

	atomic.StoreInt32(&s.chaosJobDone, 1)
	return nil

}

func stopSlaveAgentCheck(s *Server) error {
	// 4.shutdown slave and start again, the slave works well
	masterID, _ := s.getDBConnection()
	slaveID := s.getASlave()
	RunStopAgent(s.getContainerFromID(slaveID))
	RunStartAgent(s.getContainerFromID(slaveID))
	time.Sleep(5 * time.Second)

	newMasterID, _ := s.getDBConnection()
	if newMasterID != masterID {
		log.Error("new master id is not former master id after stop&start slave agent")
		os.Exit(1)
	}
	time.Sleep(5 * time.Second)

	atomic.StoreInt32(&s.chaosJobDone, 1)
	return nil
}

func partitionOneAZCheck(s *Server) error {
	// 1. (randomly) pickup one AZ, not the master AZ
	masterID, _ := s.getDBConnection()
	container, ok := s.cfg.IDContainerMapping[masterID]
	if !ok {
		log.Errorf("cannot find %s from IDContainerMapping %+v",
			masterID, s.cfg.IDContainerMapping)
		return errors.Errorf("cannot find %s from IDContainerMapping %+v",
			masterID, s.cfg.IDContainerMapping)
	}
	masterAZ := s.cfg.ContainerAZMapping[container]

	partitionedAZ := masterAZ
	for partitionedAZ == masterAZ {
		rand.Seed(time.Now().UnixNano())
		azIndex := rand.Int() % len(azSet)
		i := 0
		for az := range azSet {
			if i != azIndex {
				i++
				continue
			}
			partitionedAZ = az
			break
		}
	}
	log.Infof("the az %s will be partitioned", partitionedAZ)

	// 2. partition the picked AZ
	err := PartitionIncoming(s.cfg.PartitionTemplate, partitionedAZ, s.cfg.PartitionType)
	if err != nil {
		return err
	}
	err = PartitionOutgoing(s.cfg.PartitionTemplate, partitionedAZ, s.cfg.PartitionType)
	if err != nil {
		return err
	}

	// wait for partitioned AZ to deregister
	time.Sleep(10 * time.Second)

	atomic.StoreInt32(&s.chaosJobDone, 2)
	return nil
}

func partitionMasterAZCheck(s *Server) error {
	// 1. pickup the AZ that MySQL Master resides
	masterID, _ := s.getDBConnection()
	container, ok := s.cfg.IDContainerMapping[masterID]
	if !ok {
		log.Errorf("cannot find %s from IDContainerMapping %+v",
			masterID, s.cfg.IDContainerMapping)
		return errors.Errorf("cannot find %s from IDContainerMapping %+v",
			masterID, s.cfg.IDContainerMapping)
	}
	partitionedAZ := s.cfg.ContainerAZMapping[container]
	log.Infof("the az %s will be partitioned", partitionedAZ)

	// 2. partition the picked AZ
	go func() {
		err := PartitionIncoming(s.cfg.PartitionTemplate, partitionedAZ, s.cfg.PartitionType)
		if err != nil {
			panic(err)
		}
	}()
	go func() {
		err := PartitionOutgoing(s.cfg.PartitionTemplate, partitionedAZ, s.cfg.PartitionType)
		if err != nil {
			panic(err)
		}
	}()

	newMasterID := masterID
	for newMasterID == masterID {
		newMasterID, _ = s.getDBConnection()
		log.Infof("now newMasterID is %s and oldMasterID is %s",
			newMasterID, masterID)
		time.Sleep(1 * time.Second)
	}

	time.Sleep(5 * time.Second)

	atomic.StoreInt32(&s.chaosJobDone, 2)
	return nil
}

// singlePointMasterCheck creates the single point mode,
// and then check the cluster status
func singlePointMasterCheck(s *Server) error {

	partitionTemplate := "docker run -v /var/run/docker.sock:/var/run/docker.sock --rm -d " +
		"moiot/pumba:latest netem -d 30s "
	partitionType := "loss -p 100 "
	var err error

	newMasterID, _ := s.getDBConnection()
	var masterID string

	// partition 3 AZs, and recover twice
	// the master(s) should be dead
	for i := range []int{0, 1} {

		// spm check
		spmBytes, _, err := s.etcdClient.Get(s.ctx, "single_point_master")
		if !errors.IsNotFound(err) {
			log.Error("should not be in spm mode")
			if err != nil {
				log.Error("error is ", err)
			} else {
				log.Error("spmBytes is ", string(spmBytes))
			}
			return errors.Errorf("should not be in spm mode")
		}

		masterID = newMasterID
		for az := range azSet {
			go PartitionIncoming(partitionTemplate, az, partitionType)
			go PartitionOutgoing(partitionTemplate, az, partitionType)
		}
		for newMasterID == masterID {
			newMasterID, _ = s.getDBConnection()
			time.Sleep(1 * time.Second)
		}
		log.Infof("after %d all az partitioning, former master is %s while new master is %s",
			i+1, masterID, newMasterID)
		log.Info("sleep 30 more second for change master")
		time.Sleep(30 * time.Second)
	}

	// now the cluster enters the "SPM Mode"
	// check the status
	spmBytes, _, err := s.etcdClient.Get(s.ctx, "single_point_master")
	if newMasterID != string(spmBytes) {
		log.Errorf("single_point_master %s is not newMasterID %s", string(spmBytes), newMasterID)
		return errors.NotValidf("single_point_master %s is not newMasterID %s", string(spmBytes), newMasterID)
	}

	// if another partition occurs, master should still be working
	for az := range azSet {
		go PartitionIncoming(partitionTemplate, az, partitionType)
		go PartitionOutgoing(partitionTemplate, az, partitionType)
	}
	// wait for partition expires
	time.Sleep(30 * time.Second)
	tmpMasterID, _ := s.getDBConnection()
	if newMasterID != tmpMasterID {
		log.Errorf("after 3rd partition, master %s should not change to %s",
			newMasterID, tmpMasterID)
	}

	// bring back one mysql-node, the "SPM Mode" should be exited
	s.etcdClient.Delete(s.ctx, path.Join("election", "terms", masterID), false)
	err = RunStartAgent(s.cfg.IDContainerMapping[masterID])
	if err != nil {
		log.Errorf("error when RunStartAgent %s, %v. error: %+v",
			masterID, s.cfg.IDContainerMapping, err)
		return err
	}
	// wait for successfully bootstrap
	time.Sleep(60 * time.Second)

	slaves, err := s.etcdClient.PrefixGet(s.ctx, "/slave")
	if err != nil {
		log.Error(err)
		return errors.Trace(err)
	}
	log.Infof("slaves are %+v", slaves)
	if len(slaves) > 1 {

		// check the status of cluster
		spmBytes, _, err = s.etcdClient.Get(s.ctx, "single_point_master")
		if !errors.IsNotFound(err) {
			log.Error("get single_point_master again, result is ", string(spmBytes), err)
			return errors.NotValidf("get single_point_master again, result is ", string(spmBytes), err)
		}
	}

	atomic.StoreInt32(&s.chaosJobDone, 3)

	return nil

}
