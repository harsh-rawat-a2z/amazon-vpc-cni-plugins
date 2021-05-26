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

package network

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/aws/amazon-vpc-cni-plugins/network/vpc"
	"github.com/aws/amazon-vpc-cni-plugins/plugins/vpc-eni/network/hnswrapper"

	log "github.com/cihub/seelog"
)

const (
	// hnsNetworkNameFormat is the format of the HNS network name.
	hnsNetworkNameFormat = "task-br-%s"
	// hnsEndpointNameFormat is the format of the HNS Endpoint name.
	hnsEndpointNameFormat = "%s-ep-%s"
	// hnsTransparentNetworkType is the Type of the HNS Network created by the plugin.
	hnsTransparentNetworkType = "Transparent"
	// containerPrefix is the prefix in netns for non-infra containers.
	containerPrefix = "container:"
	// vNICNameFormat is the name format of vNIC created by Windows.
	vNICNameFormat = "vEthernet (%s)"
	// netshDisableInterface is the netsh command to disable a network interface.
	netshDisableInterface = "netsh interface set interface name=\"%s\" admin=disabled"
)

// NSType identifies the namespace type for the containers.
type NSType int

const (
	// infraContainerNS identifies an Infra container NS for networking setup.
	infraContainerNS NSType = iota
	// nonInfraContainerNS identifies sharing of infra container NS for networking setup.
	nonInfraContainerNS
	// hcsNamespace identifies HCS NS for networking setup.
	hcsNamespace
)

// netBuilder implements the Builder interface for Windows.
type netBuilder struct {
	hnswrapper hnswrapper.WindowsNetworkBuilder
}

// NewNetworkBuilder returns a new instance of NetBuilder.
func NewNetworkBuilder() Builder {
	return &netBuilder{
		hnswrapper: hnswrapper.NewWindowsNetworkBuilder(),
	}
}

// FindOrCreateNetwork creates a new HNS network.
func (nb *netBuilder) FindOrCreateNetwork(nw *Network) error {
	// Check if the HNS version is supported.
	err := nb.hnswrapper.CheckHNSVersion(hnswrapper.HnsDefaultMinVersion)
	if err != nil {
		return err
	}

	nw.Name = nb.generateHNSNetworkName(nw)
	// Create the HNS network configuration.
	networkConfig := &hnswrapper.HNSNetwork{
		Name: nw.Name,
		Type: hnsTransparentNetworkType,
	}

	if nw.ENI != nil {
		networkConfig.NetworkAdapterName = nw.ENI.GetLinkName()
	}
	if nw.IPAddress != nil {
		networkConfig.AddressPrefix = vpc.GetSubnetPrefix(nw.IPAddress).String()
	}
	if nw.GatewayIPAddress != nil {
		networkConfig.Gateway = nw.GatewayIPAddress.String()
	}

	// Find/Create the HNS network based on the config.
	existing, err := nb.hnswrapper.FindOrCreateHNSNetwork(networkConfig)
	if err != nil {
		return err
	}

	// If a new network was created then disable the management vNIC in the host compartment.
	if !existing {
		mgmtIface := fmt.Sprintf(vNICNameFormat, nw.ENI.GetLinkName())
		err = nb.disableInterface(mgmtIface)
		if err != nil {
			// This is a fatal error as the management vNIC must be disabled.
			_ = nb.DeleteNetwork(nw)
			return err
		}
	}

	return err
}

// DeleteNetwork deletes an existing HNS network.
func (nb *netBuilder) DeleteNetwork(nw *Network) error {
	// Create the network config required for network deletion.
	networkConfig := &hnswrapper.HNSNetwork{
		Name: nw.Name,
	}

	// Delete the HNS network.
	return nb.hnswrapper.DeleteHNSNetwork(networkConfig)
}

// FindOrCreateEndpoint creates a new HNS endpoint in the network.
func (nb *netBuilder) FindOrCreateEndpoint(nw *Network, ep *Endpoint) error {
	// Create the HNS endpoint config.
	hnsEndpoint := &hnswrapper.HNSEndpoint{
		Name:               "",
		VirtualNetworkName: nw.Name,
		DNSServerList:      strings.Join(nw.DNSServers, ","),
		DNSSuffix:          strings.Join(nw.DNSSuffixSearchList, ","),
		NetNS:              hnswrapper.HCSNamespace{UseNamespace: false},
		Container:          hnswrapper.AttachedContainer{IsInfraContainer: false},
	}

	if ep.MACAddress != nil {
		hnsEndpoint.MacAddress = ep.MACAddress.String()
	}
	if ep.IPAddress != nil {
		hnsEndpoint.IPAddress = ep.IPAddress
	}

	nsType, namespaceIdentifier := nb.getNamespaceIdentifier(ep)
	hnsEndpoint.Name = nb.generateHNSEndpointName(nw.Name, namespaceIdentifier)

	if nsType == hcsNamespace {
		hnsEndpoint.NetNS.UseNamespace = true
		hnsEndpoint.NetNS.NetNSName = namespaceIdentifier
	} else {
		hnsEndpoint.Container.ContainerID = ep.ContainerID
		hnsEndpoint.Container.InfraContainerID = namespaceIdentifier
		hnsEndpoint.Container.IsInfraContainer = nsType == infraContainerNS
	}

	// Create or Find the HNS endpoint.
	existing, err := nb.hnswrapper.FindOrCreateHNSEndpoint(hnsEndpoint)
	// This error means that the endpoint itself could not be created.
	if err != nil {
		return err
	}

	// Update ep and nw with the received response.
	ep.IPAddress = hnsEndpoint.IPAddress
	ep.MACAddress, _ = net.ParseMAC(hnsEndpoint.MacAddress)
	nw.GatewayIPAddress = net.ParseIP(hnsEndpoint.Gateway)

	// An existing HNS endpoint was found.
	if existing {
		if hnsEndpoint.Container.IsInfraContainer || hnsEndpoint.NetNS.UseNamespace {
			// If this endpoint is being attached to infra container/namespace of task,
			// then this would be a benign duplicate call.
			log.Infof("HNS endpoint %s is already attached to the task compartment.", hnsEndpoint.ID)
			return nil
		} else {
			// This means that we are not using namespaces and the current container is not an infra container.
			// Therefore, we should attach this endpoint to this container.
			return nb.hnswrapper.AttachEndpoint(hnsEndpoint)
		}
	}

	// Attach the HNS endpoint to namespace or container.
	err = nb.hnswrapper.AttachEndpoint(hnsEndpoint)
	if err != nil {
		// Cleanup the failed endpoint.
		nb.hnswrapper.DeleteHNSEndpoint(hnsEndpoint)
		return err
	}

	return nil
}

// DeleteEndpoint deletes an existing HNS endpoint.
func (nb *netBuilder) DeleteEndpoint(nw *Network, ep *Endpoint) error {
	// Generate network name here as endpoint is deleted before the network.
	nw.Name = nb.generateHNSNetworkName(nw)

	// Create HNS endpoint config.
	hnsEndpoint := &hnswrapper.HNSEndpoint{
		Name:      "",
		NetNS:     hnswrapper.HCSNamespace{UseNamespace: false},
		Container: hnswrapper.AttachedContainer{IsInfraContainer: false},
	}

	nsType, namespaceIdentifier := nb.getNamespaceIdentifier(ep)
	hnsEndpoint.Name = nb.generateHNSEndpointName(nw.Name, namespaceIdentifier)

	if nsType == hcsNamespace {
		hnsEndpoint.NetNS.UseNamespace = true
		hnsEndpoint.NetNS.NetNSName = ep.NetNSName
	} else {
		hnsEndpoint.Container.ContainerID = ep.ContainerID
		hnsEndpoint.Container.InfraContainerID = namespaceIdentifier
		hnsEndpoint.Container.IsInfraContainer = nsType == infraContainerNS
		// For non-infra containers, the network must not be deleted.
		if nsType == nonInfraContainerNS {
			nw.UseExisting = true
		}
	}

	// Delete the HNS endpoint.
	return nb.hnswrapper.DeleteHNSEndpoint(hnsEndpoint)
}

// generateHNSNetworkName generates a deterministic unique name for an HNS network.
func (nb *netBuilder) generateHNSNetworkName(nw *Network) string {
	if nw.UseExisting {
		return nw.Name
	}

	// Unique identifier for the network would be of format "task-br-<eni mac address>".
	id := strings.Replace(nw.ENI.GetMACAddress().String(), ":", "", -1)
	return fmt.Sprintf(hnsNetworkNameFormat, id)
}

// generateHNSEndpointName generates a deterministic unique name for the HNS Endpoint.
func (nb *netBuilder) generateHNSEndpointName(networkName string, identifier string) string {
	return fmt.Sprintf(hnsEndpointNameFormat, networkName, identifier)
}

// getNamespaceIdentifier identifies the namespace type and returns the appropriate identifier.
func (nb *netBuilder) getNamespaceIdentifier(ep *Endpoint) (NSType, string) {
	var netNSType NSType
	var namespaceIdentifier string

	if ep.NetNSName == "" || ep.NetNSName == "none" {
		// This is the first, i.e. infrastructure, container in the group.
		// The namespace identifier for such containers would be their container ID.
		netNSType = infraContainerNS
		namespaceIdentifier = ep.ContainerID
	} else if strings.HasPrefix(ep.NetNSName, containerPrefix) {
		// This is a workload container sharing the netns of a previously created infra container.
		// The namespace identifier for such containers would be the infra container's ID.
		netNSType = nonInfraContainerNS
		namespaceIdentifier = strings.TrimPrefix(ep.NetNSName, containerPrefix)
		log.Infof("Container %s shares netns of container %s.", ep.ContainerID, namespaceIdentifier)
	} else {
		// This plugin invocation does not need an infra container and uses an existing HCN Namespace.
		// The namespace identifier would be the HCN Namespace id.
		netNSType = hcsNamespace
		namespaceIdentifier = ep.NetNSName
	}

	return netNSType, namespaceIdentifier
}

// disableInterface disables the network interface with the provided name.
func (nb *netBuilder) disableInterface(adapterName string) error {
	// Check if the interface exists.
	iface, err := net.InterfaceByName(adapterName)
	if err != nil {
		return err
	}

	// Check if the interface is enabled.
	isInterfaceEnabled := strings.EqualFold(strings.Split(iface.Flags.String(), "|")[0], "up")
	if isInterfaceEnabled {
		// Disable the interface using netsh.
		log.Infof("Disabling management vNIC %s in the host namespace.", adapterName)
		commandString := fmt.Sprintf(netshDisableInterface, adapterName)
		cmd := exec.Command("cmd", "/C", commandString)

		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}
