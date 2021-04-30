// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package config

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/aws/amazon-vpc-cni-plugins/network/vpc"
	log "github.com/cihub/seelog"

	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
)

// NetConfig defines the network configuration for the vpc-eni plugin.
type NetConfig struct {
	cniTypes.NetConf
	ENIName            string
	ENIMACAddress      net.HardwareAddr
	ENIIPAddress       *net.IPNet
	GatewayIPAddress   net.IP
	NoInfraContainer   bool
	UseExistingNetwork bool
}

// netConfigJSON defines the network configuration JSON file format for the vpc-eni plugin.
type netConfigJSON struct {
	cniTypes.NetConf
	ENIName            string `json:"eniName"`
	ENIMACAddress      string `json:"eniMACAddress"`
	ENIIPAddress       string `json:"eniIPAddress"`
	GatewayIPAddress   string `json:"gatewayIPAddress"`
	NoInfraContainer   bool   `json:"noInfraContainer"`
	UseExistingNetwork bool   `json:"useExistingNetwork"`
}

// New creates a new NetConfig object by parsing the given CNI arguments.
func New(args *cniSkel.CmdArgs) (*NetConfig, error) {
	// Parse network configuration.
	var config netConfigJSON
	err := json.Unmarshal(args.StdinData, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse network config: %v", err)
	}

	// Perform validations on the received config.
	// If we are supposed to use an existing network then network name is required.
	if config.UseExistingNetwork && config.Name == "" {
		return nil, fmt.Errorf("missing required parameter network name")
	}

	// If new network creation is required, then ENI Name, Mac and ENI IP address are required.
	if !config.UseExistingNetwork {
		if config.ENIName == "" {
			return nil, fmt.Errorf("missing required parameter ENIName")
		}
		if config.ENIMACAddress == "" {
			return nil, fmt.Errorf("missing required parameter ENIMACAddress")
		}
		if config.ENIIPAddress == "" {
			return nil, fmt.Errorf("missing required parameter ENIIPAddress")
		}
	}

	// If there is no infra container then namespace id is required.
	if config.NoInfraContainer && args.Netns == "" {
		return nil, fmt.Errorf("missing required parameter netns")
	}

	// Parse the received config into NetConfig.
	netConfig := &NetConfig{
		NetConf:            config.NetConf,
		ENIName:            config.ENIName,
		NoInfraContainer:   config.NoInfraContainer,
		UseExistingNetwork: config.UseExistingNetwork,
	}

	// Parse the ENI MAC address.
	if config.ENIMACAddress != "" {
		netConfig.ENIMACAddress, err = net.ParseMAC(config.ENIMACAddress)
		if err != nil {
			return nil, fmt.Errorf("invalid ENIMACAddress %s", config.ENIMACAddress)
		}
	}

	// Parse the ENI IP address.
	if config.ENIIPAddress != "" {
		netConfig.ENIIPAddress, err = vpc.GetIPAddressFromString(config.ENIIPAddress)
		if err != nil {
			return nil, fmt.Errorf("invalid ENIIPAddress %s", config.ENIIPAddress)
		}
	}

	// Parse the optional gateway IP address.
	if config.GatewayIPAddress != "" {
		netConfig.GatewayIPAddress = net.ParseIP(config.GatewayIPAddress)
		if netConfig.GatewayIPAddress == nil {
			return nil, fmt.Errorf("invalid GatewayIPAddress %s", config.GatewayIPAddress)
		}
	}

	// Validation and parsing complete. Return the parsed NetConfig object.
	log.Debugf("Created NetConfig: %+v.", netConfig)
	return netConfig, nil
}
