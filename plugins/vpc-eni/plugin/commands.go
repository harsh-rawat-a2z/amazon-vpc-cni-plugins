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

package plugin

import (
	"github.com/aws/amazon-vpc-cni-plugins/network/eni"
	"github.com/aws/amazon-vpc-cni-plugins/plugins/vpc-eni/config"
	"github.com/aws/amazon-vpc-cni-plugins/plugins/vpc-eni/network"
	log "github.com/cihub/seelog"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurrent "github.com/containernetworking/cni/pkg/types/current"
	"github.com/pkg/errors"
)

// Add is the CNI ADD command handler.
func (plugin *Plugin) Add(args *cniSkel.CmdArgs) error {
	// Parse network configuration.
	netConfig, err := config.New(args)
	if err != nil {
		log.Errorf("Failed to parse netconfig from args: %v.", err)
		return errors.Errorf("failed to parse config: %v", err)
	}

	log.Infof("Executing ADD with netconfig: %+v ContainerID:%v Netns:%v IfName:%v Args:%v.",
		netConfig, args.ContainerID, args.Netns, args.IfName, args.Args)

	// Create the ENI.
	// We have already validated the required params during config creation. Any error at this step is redundant.
	vpcENI, _ := eni.NewENI(netConfig.ENIName, netConfig.ENIMACAddress)

	// Call the operating system specific network builder.
	nb := plugin.nb

	// Find or create the container network with the given ENI.
	nw := network.Network{
		Name:                netConfig.Name,
		ENI:                 vpcENI,
		IPAddress:           netConfig.ENIIPAddress,
		GatewayIPAddress:    netConfig.GatewayIPAddress,
		DNSServers:          netConfig.DNS.Nameservers,
		DNSSuffixSearchList: netConfig.DNS.Search,
		ShouldExist:         netConfig.UseExistingNetwork,
	}

	err = nb.FindOrCreateNetwork(&nw)
	if err != nil {
		log.Errorf("Failed to create network: %v.", err)
		return errors.Errorf("failed to create/find network: %v", err)
	}

	// Find or create the container endpoint on the network.
	ep := network.Endpoint{
		ContainerID:      args.ContainerID,
		NetNSName:        args.Netns,
		IPAddress:        netConfig.ENIIPAddress,
		MACAddress:       netConfig.ENIMACAddress,
		NoInfraContainer: netConfig.NoInfraContainer,
	}

	err = nb.FindOrCreateEndpoint(&nw, &ep)
	if err != nil {
		log.Errorf("Failed to create endpoint: %v.", err)
		return errors.Errorf("failed to create/find endpoint: %v", err)
	}

	// Generate CNI result.
	result := &cniTypesCurrent.Result{
		Interfaces: []*cniTypesCurrent.Interface{
			{
				Name:    args.IfName,
				Mac:     ep.MACAddress.String(),
				Sandbox: args.Netns,
			},
		},
		IPs: []*cniTypesCurrent.IPConfig{
			{
				Version:   "4",
				Interface: cniTypesCurrent.Int(0),
				Address:   *ep.IPAddress,
				Gateway:   nw.GatewayIPAddress,
			},
		},
	}

	// Output CNI result.
	log.Infof("Writing CNI result to stdout: %+v", result)
	err = cniTypes.PrintResult(result, netConfig.CNIVersion)
	if err != nil {
		log.Errorf("Failed to print result for CNI ADD command: %v", err)
	}

	return err
}

// Del is the CNI DEL command handler.
func (plugin *Plugin) Del(args *cniSkel.CmdArgs) error {
	// Parse network configuration.
	netConfig, err := config.New(args)
	if err != nil {
		log.Errorf("Failed to parse netconfig from args: %v.", err)
		return errors.Errorf("failed to parse config: %v", err)
	}

	log.Infof("Executing DEL with netconfig: %+v ContainerID:%v Netns:%v IfName:%v Args:%v.",
		netConfig, args.ContainerID, args.Netns, args.IfName, args.Args)

	// Create the ENI.
	// We have already validated the required params during config creation. Any error at this step is redundant.
	vpcENI, _ := eni.NewENI(netConfig.ENIName, netConfig.ENIMACAddress)

	// Call operating system specific handler.
	nb := plugin.nb

	nw := network.Network{
		Name:        netConfig.Name,
		ENI:         vpcENI,
		ShouldExist: netConfig.UseExistingNetwork,
	}

	ep := network.Endpoint{
		ContainerID:      args.ContainerID,
		NetNSName:        args.Netns,
		IPAddress:        netConfig.ENIIPAddress,
		MACAddress:       netConfig.ENIMACAddress,
		NoInfraContainer: netConfig.NoInfraContainer,
	}

	err = nb.DeleteEndpoint(&nw, &ep)
	if err != nil {
		// DEL is best-effort. Log and ignore the failure.
		log.Errorf("Failed to delete endpoint, ignoring: %v.", err)
	}

	// If we are not re-using the network, then delete it.
	if !nw.ShouldExist {
		err = nb.DeleteNetwork(&nw)
		if err != nil {
			log.Errorf("Failed to delete network: %v.", err)
			return errors.Errorf("failed to delete network: %v", err)
		}
	}

	log.Info("Execution of DEL completed.")
	return nil
}