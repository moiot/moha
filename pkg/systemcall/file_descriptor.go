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

package systemcall

import (
	"encoding/binary"
	"syscall"
)

// WriteToEventfd writes value to an given eventfd
// it is said from http://man7.org/linux/man-pages/man2/eventfd.2.html
// A write(2) fails with the error EINVAL if the size of the
// supplied buffer is less than 8 bytes, or if an attempt is made
// to write the value 0xffffffffffffffff.
func WriteToEventfd(fd int, value uint64) (int, error) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(1))
	return syscall.Write(fd, b)
}
