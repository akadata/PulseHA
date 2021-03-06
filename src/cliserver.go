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

import (
	"context"
	"encoding/json"
	"github.com/Syleron/PulseHA/proto"
	"github.com/Syleron/PulseHA/src/netUtils"
	"github.com/Syleron/PulseHA/src/utils"
	log "github.com/Sirupsen/logrus"
	"google.golang.org/grpc"
	"net"
	"sync"
	"time"
)

/**
Server struct type
*/
type CLIServer struct {
	sync.Mutex
	Server     *Server
	Listener   net.Listener
	Memberlist *Memberlist
}

/**
Attempt to join a configured cluster
Notes: We create a new client in attempt to communicate with our peer.
       If successful we acknowledge it and update our memberlist.
*/
func (s *CLIServer) Join(ctx context.Context, in *proto.PulseJoin) (*proto.PulseJoin, error) {
	log.Debug("CLIServer:Join() Join Pulse cluster")
	s.Lock()
	defer s.Unlock()
	if !gconf.ClusterCheck() {
		// Create a new client
		client := &Client{}
		// Attempt to connect
		err := client.Connect(in.Ip, in.Port, in.Hostname)
		// Handle a client connection error
		if err != nil {
			return &proto.PulseJoin{
				Success: false,
				Message: err.Error(),
			}, nil
		}
		// Create new local node config to send
		newNode := &Node{
			IP:       in.BindIp,
			Port:     in.BindPort,
			IPGroups: make(map[string][]string, 0),
		}
		// Convert struct into byte array
		buf, err := json.Marshal(newNode)
		// Handle failure to marshal config
		if err != nil {
			log.Fatal("Unable to marshal config: %s", err)
			return &proto.PulseJoin{
				Success: false,
				Message: err.Error(),
			}, nil
		}
		// Send our join request
		r, err := client.Send(SendJoin, &proto.PulseJoin{
			Config:   buf,
			Hostname: utils.GetHostname(),
		})
		// Handle a failed request
		if err != nil {
			log.Fatal("Response error: %s", err)
			return &proto.PulseJoin{
				Success: false,
				Message: err.Error(),
			}, nil
		}
		// Handle an unsuccessful request
		if !r.(*proto.PulseJoin).Success {
			log.Fatal("Peer error: %s", err)
			return &proto.PulseJoin{
				Success: false,
				Message: r.(*proto.PulseJoin).Message,
			}, nil
		}
		// Update our local config
		peerConfig := &Config{}
		err = json.Unmarshal(r.(*proto.PulseJoin).Config, peerConfig)
		// handle errors
		if err != nil {
			log.Error("Unable to unmarshal config node.")
			return &proto.PulseJoin{
				Success: false,
				Message: "Unable to unmarshal config node.",
			}, nil
		}
		// Set the config
		gconf.SetConfig(*peerConfig)
		// Save the config
		gconf.Save()
		// Reload config in memory
		gconf.Reload()
		// Setup our daemon server
		go s.Server.Setup()
		// reset our HC last received time
		localMember, _ := s.Memberlist.getLocalMember()
		localMember.setLastHCResponse(time.Now())
		// Close the connection
		client.Close()
		log.Info("Successfully joined cluster with " + in.Ip)
		return &proto.PulseJoin{
			Success: true,
			Message: "Successfully joined cluster",
		}, nil
	}
	return &proto.PulseJoin{
		Success: false,
		Message: "Unable to join as PulseHA is already in a cluster.",
	}, nil
}

/**
Break cluster / Leave from cluster
TODO: Remember to reassign active role on leave
*/
func (s *CLIServer) Leave(ctx context.Context, in *proto.PulseLeave) (*proto.PulseLeave, error) {
	log.Debug("CLIServer:Leave() - Leave Pulse cluster")
	s.Lock()
	defer s.Unlock()
	if !gconf.ClusterCheck() {
		return &proto.PulseLeave{
			Success: false,
			Message: "Unable to leave as no cluster was found",
		}, nil
	}
	// Check to see if we are not the only one in the "cluster"
	// Let everyone else know that we are leaving the cluster
	if gconf.ClusterTotal() > 1 {
		s.Memberlist.Broadcast(
			SendLeave,
			&proto.PulseLeave{
				Replicated: true,
				Hostname:   utils.GetHostname(),
			},
		)
	}
	makeMemberPassive()
	GroupClearLocal()
	NodesClearLocal()
	s.Memberlist.reset()
	gconf.Save()
	s.Server.shutdown()
	// bring down the ips
	log.Info("Successfully left configured cluster. PulseHA no longer listening..")
	if gconf.ClusterTotal() == 1 {
		return &proto.PulseLeave{
			Success: true,
			Message: "Successfully dismantled cluster",
		}, nil
	}
	return &proto.PulseLeave{
		Success: true,
		Message: "Successfully left from cluster",
	}, nil
}

/**
Note: This will probably need to be replicated..
*/
func (s *CLIServer) Create(ctx context.Context, in *proto.PulseCreate) (*proto.PulseCreate, error) {
	log.Debug("CLIServer:Create() - Create Pulse cluster")
	s.Lock()
	defer s.Unlock()
	if !gconf.ClusterCheck() {
		newNode := &Node{
			IP:       in.BindIp,
			Port:     in.BindPort,
			IPGroups: make(map[string][]string, 0),
		}
		NodeAdd(utils.GetHostname(), newNode)
		for _, ifaceName := range netUtils.GetInterfaceNames() {
			if ifaceName != "lo" {
				newNode.IPGroups[ifaceName] = make([]string, 0)
				groupName := GenGroupName()
				gconf.Groups[groupName] = []string{}
				GroupAssign(groupName, utils.GetHostname(), ifaceName)
			}
		}
		gconf.Save()
		go s.Server.Setup()
		return &proto.PulseCreate{
			Success: true,
			Message: "Pulse cluster successfully created!",
		}, nil
	} else {
		return &proto.PulseCreate{
			Success: false,
			Message: "Pulse daemon is already in a configured cluster",
		}, nil
	}
}

/**
Add a new floating IP group
Note: This will probably need to be replicated..
*/
func (s *CLIServer) NewGroup(ctx context.Context, in *proto.PulseGroupNew) (*proto.PulseGroupNew, error) {
	log.Debug("CLIServer:NewGroup() - Create floating IP group")
	s.Lock()
	defer s.Unlock()
	groupName, err := GroupNew()
	if err != nil {
		return &proto.PulseGroupNew{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	gconf.Save()
	s.Memberlist.SyncConfig()
	return &proto.PulseGroupNew{
		Success: true,
		Message: groupName + " successfully added.",
	}, nil
}

/**
Delete floating IP group
*/
func (s *CLIServer) DeleteGroup(ctx context.Context, in *proto.PulseGroupDelete) (*proto.PulseGroupDelete, error) {
	log.Debug("CLIServer:DeleteGroup() - Delete floating IP group")
	s.Lock()
	defer s.Unlock()
	err := GroupDelete(in.Name)
	if err != nil {
		return &proto.PulseGroupDelete{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	gconf.Save()
	s.Memberlist.SyncConfig()
	return &proto.PulseGroupDelete{
		Success: true,
		Message: in.Name + " successfully deleted.",
	}, nil
}

/**
Note: This will probably need to be replicated..
*/
func (s *CLIServer) GroupIPAdd(ctx context.Context, in *proto.PulseGroupAdd) (*proto.PulseGroupAdd, error) {
	log.Debug("CLIServer:GroupIPAdd() - Add IP addresses to group " + in.Name)
	s.Lock()
	defer s.Unlock()
	_, activeMember := s.Memberlist.getActiveMember()
	if activeMember == nil {
		return &proto.PulseGroupAdd{
			Success: false,
			Message: "Unable to add IP(s) to group as there no active node in the cluster.",
		}, nil
	}
	err := GroupIpAdd(in.Name, in.Ips)
	if err != nil {
		return &proto.PulseGroupAdd{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	gconf.Save()
	s.Memberlist.SyncConfig()
	// bring up the ip on the active appliance
	activeHostname, activeMember := s.Memberlist.getActiveMember()
	// Connect first just in case.. otherwise we could seg fault
	activeMember.Connect()
	configCopy := gconf.GetConfig()
	iface := configCopy.GetGroupIface(activeHostname, in.Name)
	activeMember.Send(SendBringUpIP, &proto.PulseBringIP{
		Iface: iface,
		Ips: in.Ips,
	})
	// respond
	return &proto.PulseGroupAdd{
		Success: true,
		Message: "IP address(es) successfully added to " + in.Name,
	}, nil
}

/**
Note: This will probably need to be replicated..
*/
func (s *CLIServer) GroupIPRemove(ctx context.Context, in *proto.PulseGroupRemove) (*proto.PulseGroupRemove, error) {
	log.Debug("CLIServer:GroupIPRemove() - Removing IPs from group " + in.Name)
	s.Lock()
	defer s.Unlock()
	// TODO: Note: Validation! IMPORTANT otherwise someone could DOS by seg faulting.
	if in.Ips == nil || in.Name == "" {
		return &proto.PulseGroupRemove{
			Success: false,
			Message: "Unable to process RPC call. Required parameters: Ips, Name",
		}, nil
	}
	_, activeMember := s.Memberlist.getActiveMember()
	if activeMember == nil {
		return &proto.PulseGroupRemove{
			Success: false,
			Message: "Unable to remove IP(s) to group as there no active node in the cluster.",
		}, nil
	}
	err := GroupIpRemove(in.Name, in.Ips)
	if err != nil {
		return &proto.PulseGroupRemove{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	gconf.Save()
	s.Memberlist.SyncConfig()
	// bring down the ip on the active appliance
	activeHostname, activeMember := s.Memberlist.getActiveMember()
	// Connect first just in case.. otherwise we could seg fault
	activeMember.Connect()
	configCopy := gconf.GetConfig()
	iface := configCopy.GetGroupIface(activeHostname, in.Name)
	activeMember.Send(SendBringDownIP, &proto.PulseBringIP{
		Iface: iface,
		Ips: in.Ips,
	})
	return &proto.PulseGroupRemove{
		Success: true,
		Message: "IP address(es) successfully removed from " + in.Name,
	}, nil
}

/**
Note: This will probably need to be replicated..
*/
func (s *CLIServer) GroupAssign(ctx context.Context, in *proto.PulseGroupAssign) (*proto.PulseGroupAssign, error) {
	log.Debug("CLIServer:GroupAssign() - Assigning group " + in.Group + " to interface " + in.Interface + " on node " + in.Node)
	s.Lock()
	defer s.Unlock()
	err := GroupAssign(in.Group, in.Node, in.Interface)
	if err != nil {
		return &proto.PulseGroupAssign{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	gconf.Save()
	s.Memberlist.SyncConfig()
	return &proto.PulseGroupAssign{
		Success: true,
		Message: in.Group + " assigned to interface " + in.Interface + " on node " + in.Node,
	}, nil
}

/**
Note: This will probably need to be replicated..
*/
func (s *CLIServer) GroupUnassign(ctx context.Context, in *proto.PulseGroupUnassign) (*proto.PulseGroupUnassign, error) {
	log.Debug("CLIServer:GroupUnassign() - Unassigning group " + in.Group + " from interface " + in.Interface + " on node " + in.Node)
	s.Lock()
	defer s.Unlock()
	err := GroupUnassign(in.Group, in.Node, in.Interface)
	if err != nil {
		return &proto.PulseGroupUnassign{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	gconf.Save()
	s.Memberlist.SyncConfig()
	return &proto.PulseGroupUnassign{
		Success: true,
		Message: in.Group + " unassigned from interface " + in.Interface + " on node " + in.Node,
	}, nil
}

/**

 */
func (s *CLIServer) GroupList(ctx context.Context, in *proto.GroupTable) (*proto.GroupTable, error) {
	log.Debug("CLIServer:GroupList() - Getting groups and their IPs")
	s.Lock()
	defer s.Unlock()
	table := new(proto.GroupTable)
	config := gconf.GetConfig()
	for name, ips := range config.Groups {
		nodes, interfaces := getGroupNodes(name)
		row := &proto.GroupRow{Name: name, Ip: ips, Nodes: nodes, Interfaces: interfaces}
		table.Row = append(table.Row, row)
	}
	return table, nil
}

/**
Return the status for each node within the cluster
*/
func (s *CLIServer) Status(ctx context.Context, in *proto.PulseStatus) (*proto.PulseStatus, error) {
	log.Debug("CLIServer:Status() - Getting cluster node statuses")
	s.Lock()
	defer s.Unlock()
	table := new(proto.PulseStatus)
	for _, member := range s.Memberlist.Members {
		details, _ := NodeGetByName(member.Hostname)
		tym := member.getLastHCResponse()
		var tymFormat string
		if tym == (time.Time{}) {
			tymFormat = ""
		} else {
			tymFormat = tym.Format(time.RFC1123)
		}
		row := &proto.StatusRow{
			Hostname: member.getHostname(),
			Ip:       details.IP,
			Latency:     member.getLatency(),
			Status:   member.getStatus(),
			LastReceived: tymFormat,
		}
		table.Row = append(table.Row, row)
	}
	return table, nil
}

/**
Handle CLI promote request
 */
func (s *CLIServer) Promote(ctx context.Context, in *proto.PulsePromote) (*proto.PulsePromote, error) {
	log.Debug("CLIServer:Promote() - Promote a new member")
	s.Lock()
	defer s.Unlock()
	err := s.Memberlist.PromoteMember(in.Member)
	if err != nil {
		return &proto.PulsePromote{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return &proto.PulsePromote{
		Success: true,
		Message: "Successfully promoted member " + in.Member,
	}, nil
}

/**
Setup pulse cli type
*/
func (s *CLIServer) Setup() {
	log.Info("CLI server initialised on 127.0.0.1:9443")
	lis, err := net.Listen("tcp", "127.0.0.1:9443")
	if err != nil {
		log.Errorf("Failed to listen: %s", err)
	}
	grpcServer := grpc.NewServer()
	proto.RegisterCLIServer(grpcServer, s)
	grpcServer.Serve(lis)
}
