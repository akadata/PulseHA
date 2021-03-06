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
	"github.com/Syleron/PulseHA/src/utils"
	log "github.com/Sirupsen/logrus"
	"os"
	"path"
	"path/filepath"
	"plugin"
	"strconv"
)

/**
Health Check plugin type
 */
type PluginHC interface {
	Name() string
	Version() float64
	Send() (bool, bool)
}

/**
Networking plugin type
 */
type PluginNet interface {
	Name() string
	Version() float64
	BringUpIPs(iface string, ips []string) error
	BringDownIPs(iface string, ips []string) error
}

/**
Plugins struct
 */
type Plugins struct {
	modules []*Plugin
}

/**
Struct for a specific plugin
 */
type Plugin struct {
	Name string
	Version float64
	Type interface{}
	Plugin interface{}
}

type pluginType int

const (
	PluginHealthCheck pluginType = 1 + iota
	PluginNetworking
)

var pluginTypeNames = []string{
	"PluginHC",
	"PluginNet",
}

func (p pluginType) String() string {
	return pluginTypeNames[p-1]
}

/**
Define each type of plugin to load
 */
func (p *Plugins) Setup() {
	// Get the project directory absolute path
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	// Handle the error
	if err != nil {
		panic(err)
	}
	// Create plugin folder
	utils.CreateFolder(dir + "/plugins")
	// Join any number of file paths into a single path
	evtGlob := path.Join(dir + "/plugins", "/*.so")
	// Return all the files that match the file name pattern
	evt, err := filepath.Glob(evtGlob)
	// handle errors
	if err != nil {
		panic(err.Error())
	}
	// list of plugins
	var plugins []*plugin.Plugin
	// Load them
	for _, pFile := range evt {
		if plug, err := plugin.Open(pFile); err == nil {
			plugins = append(plugins, plug)
		} else {
			log.Warning("Unable to load plugin " + pFile + ". Perhaps it is out of date?")
			log.Debug(pFile + " - " + err.Error())
		}
	}
	p.Load(PluginHealthCheck, plugins)
	p.Load(PluginNetworking, plugins)
	p.validate()
	if len(p.modules) > 0 {
		var pluginNames string = ""
		for _, plgn := range p.modules {
			pluginNames += plgn.Name + "(v" + strconv.FormatFloat(plgn.Version, 'f', -1, 32) + ") "
		}
		log.Infof("Plugins loaded (%v): %v", len(p.modules), pluginNames)
	}
}

/**

 */
func (p *Plugins) validate() {
	// make sure we have a networking plugin
	if p.getNetworkingPlugin() == nil {
		log.Fatal("No networking plugin loaded. Please install a networking plugin in order to use PulseHA")
	}
}

func (p *Plugins) Load(pluginType pluginType, pluginList []*plugin.Plugin) {
	// TODO: Note: Unfortunately a switch statement must be used as you cannot dynamically typecast a variable.
	for _, plugin := range pluginList	 {
		switch pluginType {
		case PluginHealthCheck:
			symEvt, err := plugin.Lookup(pluginType.String())
			if err != nil {
				log.Debugf("Plugin does not match pluginType symbol: %v", err)
				continue
			}
			e, ok := symEvt.(PluginHC)
			if !ok {
				continue
			}
			// Create a new instance of plugins
			newPlugin := &Plugin{
				Name: e.Name(),
				Type: pluginType,
			}
			// Add to the list of plugins
			p.modules = append(p.modules, newPlugin)
		case PluginNetworking:
			// Make sure we are not loading another networking plugin.
			// Only one networking plugin can be loaded at one time.
			if p.getNetworkingPlugin() != nil {
				continue
			}
			symEvt, err := plugin.Lookup(pluginType.String())
			if err != nil {
				log.Debugf("Plugin does not match pluginType symbol: %v", err)
				continue
			}
			e, ok := symEvt.(PluginNet)
			if !ok {
				continue
			}
			// Create a new instance of plugins
			newPlugin := &Plugin{
				Name: e.Name(),
				Version: e.Version(),
				Type: pluginType,
				Plugin: e,
			}
			// Add to the list of plugins
			p.modules = append(p.modules, newPlugin)
		}
	}
}

/**
Returns a slice of health check plugins
 */
func (p *Plugins) getHealthCheckPlugins() []*Plugin {
	modules := []*Plugin{}
	for _, plgin := range p.modules {
		if plgin.Type == PluginHealthCheck {
			modules = append(modules, plgin)
		}
	}
	return modules
}

/**
Returns a single networking plugin (as you should only ever have one loaded)
 */
func (p *Plugins) getNetworkingPlugin() *Plugin {
	for _, plgin := range p.modules {
		if plgin.Type == PluginNetworking {
			return plgin
		}
	}
	return nil
}
