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

package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/juju/errors"
	"github.com/moiot/moha/pkg/log"
)

func (s *Server) initHTTPServer() (*http.Server, error) {
	listenURL, err := url.Parse(s.cfg.ListenAddr)
	if err != nil {
		return nil, errors.Annotate(err, "fail to parse listen address")
	}

	// listen & serve.
	httpSrv := &http.Server{Addr: fmt.Sprintf(":%s", listenURL.Port())}
	// TODO add the new status function to detect all agents' status
	// http.HandleFunc("/status", s.Status)
	http.HandleFunc("/changeMaster", s.ChangeMaster)
	http.HandleFunc("/setReadOnly", s.SetReadOnly)
	http.HandleFunc("/setReadWrite", s.SetReadWrite)
	http.HandleFunc("/setOnlyFollow", s.SetOnlyFollow)
	http.HandleFunc("/masterCheck", s.MasterCheck)
	http.HandleFunc("/slaveCheck", s.SlaveCheck)
	http.HandleFunc("/status", s.Status)
	return httpSrv, nil
}

// MasterCheck return status 200 iff current node is master else status 418
func (s *Server) MasterCheck(w http.ResponseWriter, r *http.Request) {

	if s.amILeader() {
		w.Write([]byte("ok"))
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, http.StatusText(http.StatusTeapot), http.StatusTeapot)
	}
}

// SlaveCheck return status 200 iff current node is not master else status 418
func (s *Server) SlaveCheck(w http.ResponseWriter, r *http.Request) {

	if !s.amILeader() || s.amISPM() {
		w.Write([]byte("ok"))
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, http.StatusText(http.StatusTeapot), http.StatusTeapot)
	}
}

// ChangeMaster triggers change master by endpoint
func (s *Server) ChangeMaster(w http.ResponseWriter, r *http.Request) {
	if r != nil {
		log.Info("receive ChangeMaster from ", r.RemoteAddr)
	}
	if s.amILeader() {
		// set service readonly to avoid brain-split
		s.setServiceReadonlyOrShutdown()
		// delete leader key
		s.deleteLeaderKey()
		// wait so that other agents are more likely to be the leader
		time.Sleep(500 * time.Millisecond)
		// inform leaderStopCh to stop keeping alive loop
		close(s.leaderStopCh)
		log.Info("current node is leader and has given up leader successfully")
		w.Write([]byte("current node is leader and has given up leader successfully\n"))
	} else {
		log.Info("current node is not leader, change master should be applied on leader")
		w.Write([]byte("current node is not leader, change master should be applied on leader\n"))
	}
}

// SetReadOnly sets mysql readonly
func (s *Server) SetReadOnly(w http.ResponseWriter, r *http.Request) {
	log.Info("receive setReadOnly request")
	if s.amILeader() {
		success, err := s.serviceManager.SetReadOnlyManually()
		if err != nil {
			log.Info("set current node readonly fail")
			log.Error("error when set current node readonly ", err)
			w.Write([]byte("set current node readonly fail\n"))
			w.Write([]byte("error when set current node readonly" + err.Error() + "\n"))
			return
		}
		if success {
			log.Info("set current node readonly success")
			w.Write([]byte(fmt.Sprint("set current node readonly success\n")))
		} else {
			log.Info("set current node readonly fail")
			w.Write([]byte(fmt.Sprint("set current node readonly fail\n")))
		}
		return
	}
	log.Info("set current node readonly fail")
	log.Info("current node is not leader, so no need to set readonly")
	w.Write([]byte("set current node readonly fail\n"))
	w.Write([]byte("current node is not leader, so no need to set readonly\n"))
}

// SetReadWrite sets mysql readwrite
func (s *Server) SetReadWrite(w http.ResponseWriter, r *http.Request) {
	log.Info("receive setReadWrite request")
	if s.amILeader() {
		err := s.serviceManager.SetReadWrite()
		if err != nil {
			log.Info("set current node readwrite fail")
			log.Error("error when set current node readwrite ", err)
			w.Write([]byte("set current node readwrite fail\n"))
			w.Write([]byte("error when set current node readwrite" + err.Error() + "\n"))
			return
		}
		log.Info("set current node readwrite success")
		w.Write([]byte("set current node readwrite success\n"))
		return
	}
	log.Info("set current node readwrite fail")
	log.Info("current node is not leader, so no need to set readwrite")
	w.Write([]byte("set current node readwrite fail\n"))
	w.Write([]byte("current node is not leader, so no need to set readwrite\n"))
}

// SetOnlyFollow sets the s.onlyFollow to true or false,
// depending on the param `onlyFollow` passed in
func (s *Server) SetOnlyFollow(w http.ResponseWriter, r *http.Request) {
	log.Info("receive setOnlyFollow request")
	operation, ok := r.URL.Query()["onlyFollow"]
	if !ok || len(operation) < 1 {
		log.Errorf("has no onlyFollow in query %+v ", r.URL.Query())
		w.Write([]byte(fmt.Sprintf("has no onlyFollow in query %+v ", r.URL.Query())))
		return
	}
	switch operation[0] {
	case "true":
		log.Info("set onlyFollow to true")
		s.onlyFollow = true
		w.Write([]byte("set onlyFollow to true"))
		break
	case "false":
		log.Info("set onlyFollow to false")
		s.onlyFollow = false
		w.Write([]byte("set onlyFollow to false"))
		break
	default:
		log.Info("set onlyFollow operation is undefined ", operation[0])
		w.Write([]byte("set onlyFollow operation is undefined " + operation[0]))
	}
}

// Status prints the status of current node
func (s *Server) Status(w http.ResponseWriter, r *http.Request) {
	status := s.getServerStatus()

	encoder := json.NewEncoder(w)
	encoder.Encode(status)

}
