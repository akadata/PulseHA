/*
   PulseHA - HA Cluster Daemon
   Copyright (C) 2017  Andrew Zak <andrew@pulseha.com>

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/
package utils

import (
	"errors"
	log "github.com/Sirupsen/logrus"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"
)

/**
 * Load a specific file and return byte code
 **/
func LoadFile(file string) []byte {
	c, err := ioutil.ReadFile(file)

	// We had an error attempting to decode the json into our struct! oops!
	if err != nil {
		//log.Error("Unable to load file. Does it exist?")
		os.Exit(1)
	}

	return []byte(c)
}

/**
 * Execute system command.
 */
func Execute(cmd string, args ...string) (string, error) {
	command := exec.Command(cmd, args...)

	//printCommand(command)
	output, err := command.CombinedOutput()

	if err != nil {
		//printError(err)
		return "", err
	}

	return string(output), err
}

/**
 * Function that validates an IPv4 and IPv6 address.
 *
 * @return bool
 */
func ValidIPAddress(ipAddress string) error {
	ip, _, err := net.ParseCIDR(ipAddress)
	if err != nil {
		return errors.New("invalid CDIR address specified")
	}
	testInput := net.ParseIP(ip.String())
	if testInput.To4() == nil {
		return errors.New("invalid IP address")
	}
	return nil
}

/**
 * Function to schedule the execution every x time as time.Duration.
 */
func Scheduler(method func() bool, delay time.Duration) {
	for _ = range time.Tick(delay) {
		end := method()
		if end {
			break
		}
	}
}

/**
 * Create folder if it doesn't already exist!
 * Returns true or false depending on whether the folder was created or not.
 */
func CreateFolder(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.Mkdir(path, os.ModePerm)
		return true
	}
	return false
}

/**
 * Check if a folder exists.
 */
func CheckFolderExist(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

/**
 * Get local hostname
 * Note: This may break with FQDs
 */
func GetHostname() string {
	output, err := Execute("hostname")
	if err != nil {
		log.Error("Failed to obtain hostname.")
		os.Exit(1)
	}
	// Remove new line characters
	return strings.TrimSuffix(output, "\n")
}

/**
 * Function to return an IP and Port from a single ip:port string
 */
func SplitIpPort(ipPort string) (string, string, error) {
	IPslice := strings.Split(ipPort, ":")

	if len(IPslice) < 2 {
		return "", "", errors.New("Invalid IP:Port string. Unable to split.")
	}

	return IPslice[0], IPslice[1], nil
}

/**
Checks if a value exists inside of a slice
*/
func in_array(val interface{}, array interface{}) (exists bool, index int) {
	exists = false
	index = -1
	switch reflect.TypeOf(array).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(array)
		for i := 0; i < s.Len(); i++ {
			if reflect.DeepEqual(val, s.Index(i).Interface()) == true {
				index = i
				exists = true
				return
			}
		}
	}
	return
}
