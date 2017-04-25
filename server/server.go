package server

import (
	log "github.com/Sirupsen/logrus"
	"net"
	hc "github.com/syleron/pulse/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"sync"
	"github.com/syleron/pulse/structures"
	"github.com/syleron/pulse/utils"
	"google.golang.org/grpc/codes"
	"time"
	"github.com/syleron/pulse/client"
)

var (
	Config	structures.Configuration
	Role	string

	ServerIP string
	ServerPort string

	Last_response time.Time // Last time we got a health check from the master
	Status hc.HealthCheckResponse_ServingStatus // The status of the cluster
)

type server struct{
	mu sync.Mutex
	status hc.HealthCheckResponse_ServingStatus
}

func (s *server) Check(ctx context.Context, in *hc.HealthCheckRequest) (*hc.HealthCheckResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch(in.Request) {
	case hc.HealthCheckRequest_SETUP:
		log.Info("Recieved setup request from master..")

		if (configureCluster()) {
			// Reset the last_response time
			Last_response = time.Now()
			// Successfully configured the cluster... now to monitor for health checks
			go monitorResponses()
			// We return unknown as the request was not successful.
			return &hc.HealthCheckResponse{
				Status: hc.HealthCheckResponse_CONFIGURED,
			}, nil
		} else {
			// We return unknown as the request was not successful.
			return nil, grpc.Errorf(codes.PermissionDenied, "Slave has already been configured.")
			//return &hc.HealthCheckResponse{
			//	Status: hc.HealthCheckResponse_UNKNOWN,
			//}, nil
		}
	case hc.HealthCheckRequest_STATUS:
		// Make sure we are configured
		if Config.Local.Configured {
			// Reset the last_response time
			Last_response = time.Now()

			return &hc.HealthCheckResponse{
				Status: hc.HealthCheckResponse_HEALTHY,
			}, nil
		} else {
			return nil, grpc.Errorf(codes.PermissionDenied, "A setup request must be made before the slave can respond to health checks.")
		}
	default:
		return nil, grpc.Errorf(codes.NotFound, "unknown request")
	}
}

/**
 * Function is used to configure a clustered pair
 */
func configureCluster() bool{
	// Check to see if we can configure this node
	// make sure we are a slave
	if Config.Local.Role == "slave" {
		// Are we in a configured state already?
		if Config.Local.Configured == false {
			// Set the local value to configured
			Config.Local.Configured = true;

			// Save
			utils.SaveConfig(Config)

			log.Info("Succesfully configured slave.")

			return true;
		} else {
			return false
		}
	}
	return false
}

/*
 * Setup Function used to initialise the server
 */
func Setup(wg *sync.WaitGroup) {
	// Load the config and validate
	Config = utils.LoadConfig()
	Config.Validate()

	// Are we master or slave?

	// Setup local variables
	setupLocalVariables()

	// Log message
	log.Info(Role + " initialised on port " + ServerPort);

	defer wg.Done()

	lis, err := net.Listen("tcp", ":" + ServerPort)

	if err != nil {
		log.Error("failed to listen: %v", err)
	}

	s := grpc.NewServer()

	hc.RegisterRequesterServer(s, &server{})

	// If we are a slave.. we need to set the starting time
	// and fail-over checker
	// Note: as go routine otherwise the server doesn't serve!
	go func() {
		if Config.Local.Role == "slave" && Config.Local.Configured {
			Last_response = time.Now()

			// TODO: this time needs to change
			monitorResponses()
		}
	}()

	s.Serve(lis)
}

/**
 * Slave function - used to monitor when the last health check we received.
 */
func monitorResponses() {
	for _ = range time.Tick(time.Duration(Config.Local.FOCInterval) * time.Millisecond) {
		elapsed := int64(time.Since(Last_response)) / 1e9
		
		if int(elapsed) > 0 && int(elapsed)%4 == 0 {
			log.Warn("No healthchecks are being made.. Perhaps a failover is required?")
		}

		// If 30 seconds has gone by.. something is wrong.
		if int(elapsed) >= Config.Local.FOCLimit {
			// Try communicating with the master through other methods

			// Attempt ICMP Health check
			// Attempt HTTP Health Check
			// NOTE: use a waitgroup with go routines perhaps? this way they can be sent at the same time

			// Nothing has worked.. assume the master has failed. Fail over.
			log.Info("Attempting a failover..")
			failover()
			break
		}
	}
}

/**
 * Slave Function - Used when the master is no longer around.
 */
func failover() {
	if (Config.Local.Role == "slave") {
		// update local role
		Config.Local.Role = "master"

		// Update local status
		Status = hc.HealthCheckResponse_FAILVER

		// Update network setting
		// -- Bring up floating IP

		// Save to file
		utils.SaveConfig(Config)

		// Tell the client to reload the config
		client.ForceConfigReload()

		log.Info("Completed. Local role has been re-assigned as master..")
	}
}

func setupLocalVariables() {
	switch Config.Local.Role {
	case "master":
		ServerIP= Config.Cluster.Nodes.Master.IP
		ServerPort = Config.Cluster.Nodes.Master.Port
		Role = "master"
	case "slave":
		ServerIP = Config.Cluster.Nodes.Slave.IP
		ServerPort = Config.Cluster.Nodes.Slave.Port
		Role = "slave"
	default:
		panic("Unable to initiate due to invalid role set in configuration.")
	}

	// Local configuration status
	if (Config.Local.Configured) {
		Status = hc.HealthCheckResponse_CONFIGURED
	} else {
		Status = hc.HealthCheckResponse_UNCONFIGURED
	}
}