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

package file

import (
	"errors"
	"os"
	"syscall"
)

const (
	// PrivateFileMode is the default permission for file
	PrivateFileMode = 0600

	// PrivateDirMode is the default permission for dir
	PrivateDirMode = 0700
)

var (
	// ErrLocked returns error when fail to get file lock.
	ErrLocked = errors.New("file: file already locked")
)

// LockedFile is a wrapper of file with lock logic.
type LockedFile struct{ *os.File }

// TryLockFile tries to open the file with a file lock.
func TryLockFile(path string, flag int, perm os.FileMode) (*LockedFile, error) {
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			err = ErrLocked
		}
		return nil, err
	}
	return &LockedFile{f}, nil
}
