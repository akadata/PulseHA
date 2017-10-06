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
package main

import  (
	"sync"
	"github.com/coreos/go-log/log"
	"github.com/Syleron/PulseHA/proto"
)
/**
 * Member struct type
 */
type Member struct {
	hostname   string
	status proto.MemberStatus_Status
	Client
	sync.Mutex
}

/*
 Getters and setters for Member which allow us to make them go routine safe
 */

func (m *Member) getHostname()string {
	m.Lock()
	defer m.Unlock()
	return m.hostname
}

func (m *Member) setHostname(hostname string){
	m.Lock()
	defer m.Unlock()
	m.hostname = hostname
}

func (m *Member) getStatus()proto.MemberStatus_Status {
	m.Lock()
	defer m.Unlock()
	return m.status
}

func (m *Member) setStatus(status proto.MemberStatus_Status) {
	m.Lock()
	defer m.Unlock()
	m.status = status
}
func (m *Member) setClient(client Client) {
	m.Client = client
}

/*
	Make the node active (bring up its groups)
 */
func (m *Member) makeActive()bool{
	log.Debugf("Making active %s", m.getHostname())

	if m.hostname == gconf.getLocalNode() {
		log.Debug("member is local node making active")
		makeMemberActive()
	} else {
		log.Debug("member is not localnode making grpc call")
		err := m.SendMakeActive(&proto.PulsePromote{Success:false, Message:"", Member: m.getHostname()})
		if err != nil {
			log.Error(err)
			log.Errorf("Error making %s active. Error: %s", m.getHostname(), err.Error())
			return false
		}
	}
	m.status = proto.MemberStatus_ACTIVE
	return true
}

/**
	Make the node passive (take down its groups)
 */
func (m *Member) makePassive()bool {
	log.Debugf("Making passive %s", m.getHostname())
	if m.hostname == gconf.getLocalNode() {
		log.Debug("member is local node making active")
		makeMemberPassive()
	} else {
		log.Debug("member is not localnode making grpc call")
		err := m.SendMakePassive(&proto.PulsePromote{Success:false, Message:"", Member: m.getHostname()})
		if err != nil {
			log.Error(err)
			log.Errorf("Error making %s passive. Error: %s", m.getHostname(), err.Error())
			return false
		}
	}
	m.status = proto.MemberStatus_PASSIVE
	return true
}
/**
	Used to bring up a single IP on member
	We need to know the group to work out what interface to
	bring it up on.
 */
func (m *Member)bringUpIPs(ips []string,group string)bool{
	configCopy := gconf.GetConfig()
	iface := configCopy.GetGroupIface(m.hostname, group)
	if m.hostname == gconf.getLocalNode() {
		log.Debug("member is local node bringing up IP's")
		bringUpIPs(iface,ips)
	} else {
		log.Debug("member is not localnode making grpc call")
		err := m.SendBringUpIPs(&proto.PulseBringIP{Iface:iface, Ips:ips})
		if err != nil {
			log.Error(err)
			log.Errorf("Error making %s passive. Error: %s", m.getHostname(), err.Error())
			return false
		}
	}
	m.status = proto.MemberStatus_PASSIVE
	return true
}