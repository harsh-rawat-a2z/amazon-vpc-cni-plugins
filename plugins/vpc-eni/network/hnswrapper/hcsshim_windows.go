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

package hnswrapper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/Microsoft/hcsshim"
	log "github.com/cihub/seelog"
)

type builder struct{}

// NewWindowsNetworkBuilder returns a new instance of NetworkBuilder for Windows.
func NewWindowsNetworkBuilder() WindowsNetworkBuilder {
	return &builder{}
}

// CheckHNSVersion returns whether the Windows Host Networking Service version is supported.
func (builder *builder) CheckHNSVersion(hnsMinVersion hcsshim.HNSVersion) error {

	hnsGlobals, err := hcsshim.GetHNSGlobals()
	if err != nil {
		return err
	}

	hnsVersion := hnsGlobals.Version
	log.Infof("Running on HNS version: %+v.", hnsVersion)

	supported := hnsVersion.Major > hnsMinVersion.Major ||
		(hnsVersion.Major == hnsMinVersion.Major && hnsVersion.Minor >= hnsMinVersion.Minor)

	if !supported {
		return fmt.Errorf("HNS is older than the minimum supported version %v", hnsMinVersion)
	}

	return nil
}

// FindOrCreateHNSNetwork creates a new HNS network if an existing network is not found.
func (builder *builder) FindOrCreateHNSNetwork(nw *HNSNetwork) (bool, error) {
	var existingNetwork = false

	// Find the HNS Network using its name.
	hnsNetwork, err := hcsshim.GetHNSNetworkByName(nw.Name)
	if err == nil {
		log.Infof("Found existing HNS network %s.", nw.Name)
		existingNetwork = true
		return existingNetwork, nil
	}

	// Return an error if an existing network was to be used but is not available.
	if nw.ShouldExist {
		log.Errorf("Could not find an existing HNS network: %v.", err)
		return existingNetwork, err
	}

	// Create config for new HNS network.
	hnsNetwork = &hcsshim.HNSNetwork{
		Name:               nw.Name,
		Type:               nw.Type,
		NetworkAdapterName: nw.NetworkAdapterName,
		Subnets: []hcsshim.Subnet{
			{
				AddressPrefix:  nw.AddressPrefix,
				GatewayAddress: nw.Gateway,
			},
		},
	}

	buf, err := json.Marshal(hnsNetwork)
	if err != nil {
		return existingNetwork, err
	}
	hnsRequest := string(buf)

	// Create the HNS network.
	log.Infof("Creating HNS network: %+v.", hnsRequest)
	hnsResponse, err := hcsshim.HNSNetworkRequest("POST", "", hnsRequest)
	if err != nil {
		log.Errorf("Failed to create HNS network: %v.", err)
		return existingNetwork, err
	}

	log.Infof("Received HNS network response: %+v.", hnsResponse)

	return existingNetwork, nil
}

// DeleteHNSNetwork deletes the HNS network.
func (builder *builder) DeleteHNSNetwork(nw *HNSNetwork) error {
	// Find the HNS network using its name.
	hnsNetwork, err := hcsshim.GetHNSNetworkByName(nw.Name)
	if err != nil {
		log.Errorf("Failed to delete HNS network: %v.", err)
		return err
	}

	// Delete the HNS network.
	log.Infof("Deleting HNS network name: %s ID: %s.", nw.Name, hnsNetwork.Id)
	_, err = hcsshim.HNSNetworkRequest("DELETE", hnsNetwork.Id, "")
	if err != nil {
		log.Errorf("Failed to delete HNS network: %v.", err)
	}

	return err
}

// FindOrCreateHNSEndpoint creates a new HNS endpoint if an existing endpoint is not found.
func (builder *builder) FindOrCreateHNSEndpoint(ep *HNSEndpoint) (bool, error) {
	var existingEndpoint = false

	// Find the HNS Endpoint using its name.
	hnsEndpoint, err := hcsshim.GetHNSEndpointByName(ep.Name)
	if err == nil {
		//Found existing endpoint which we will return.
		log.Infof("Found existing HNS endpoint: %s.", ep.Name)
		builder.populateEndpointFieldsFromResponse(hnsEndpoint, ep)
		existingEndpoint = true
		return existingEndpoint, nil
	}

	// If we are not using namespaces and this is not a infra container, then do not create an endpoint.
	if !ep.Container.IsInfraContainer && !ep.NetNS.UseNamespace {
		log.Errorf("Failed to find endpoint %s for the namespace.", ep.Name)
		return existingEndpoint, fmt.Errorf("failed to find endpoint %s: %v", ep.Name, err)
	}

	// Initialize the HNS endpoint.
	hnsEndpoint = &hcsshim.HNSEndpoint{
		Name:               ep.Name,
		VirtualNetworkName: ep.VirtualNetworkName,
		DNSServerList:      ep.DNSServerList,
		DNSSuffix:          ep.DNSSuffix,
		GatewayAddress:     ep.Gateway,
		MacAddress:         ep.MacAddress,
	}

	if ep.IPAddress != nil {
		hnsEndpoint.IPAddress = ep.IPAddress.IP
		pl, _ := ep.IPAddress.Mask.Size()
		hnsEndpoint.PrefixLength = uint8(pl)
	}

	// Attach policies associated with the endpoint.
	hnsEndpoint.Policies = ep.Policies

	// Encode the endpoint request.
	buf, err := json.Marshal(hnsEndpoint)
	if err != nil {
		return existingEndpoint, err
	}
	hnsRequest := string(buf)

	// Create the HNS endpoint.
	log.Infof("Creating HNS endpoint: %+v.", hnsRequest)
	hnsResponse, err := hcsshim.HNSEndpointRequest("POST", "", hnsRequest)
	if err != nil {
		log.Errorf("Failed to create HNS endpoint: %v.", err)
		return existingEndpoint, err
	}

	log.Infof("Received HNS endpoint response: %+v.", hnsResponse)

	builder.populateEndpointFieldsFromResponse(hnsResponse, ep)
	return existingEndpoint, nil
}

// DeleteHNSEndpoint deletes an HNS endpoint.
func (builder *builder) DeleteHNSEndpoint(ep *HNSEndpoint) error {
	if ep.NetNS.UseNamespace {
		return builder.deleteHNSEndpointV2(ep)
	} else {
		return builder.deleteHNSEndpointV1(ep)
	}
}

// AttachEndpoint attaches an HNS endpoint to the network namespace.
func (builder *builder) AttachEndpoint(ep *HNSEndpoint) error {
	if ep.NetNS.UseNamespace {
		return builder.attachEndpointV2(ep)
	} else {
		return builder.attachEndpointV1(ep)
	}
}

// CreatePolicy creates a new policy and attaches it to the endpoint.
// Currently, only HNS Route policy and HNS Outbound NAT policies can be created using this API.
func (builder *builder) CreatePolicy(ep *HNSEndpoint, policy interface{}) error {
	switch policy.(type) {
	case HnsRoutePolicy:
		policy := policy.(HnsRoutePolicy)
		policy.Policy = hcsshim.Policy{Type: hcsshim.Route}

	case HnsOutboundNATPolicy:
		policy := policy.(HnsOutboundNATPolicy)
		policy.Policy = hcsshim.Policy{Type: hcsshim.OutboundNat}

	default:
		log.Error("Failed to create unsupported policy type.")
		return errors.New("failed to create unsupported policy type")
	}

	buf, err := json.Marshal(policy)
	if err != nil {
		log.Errorf("Failed to encode policy: %v.", err)
		return err
	}

	ep.Policies = append(ep.Policies, buf)
	return nil
}

// populateEndpointFieldsFromResponse populates the fields in HNSEndpoint from the response received from HNS.
func (builder *builder) populateEndpointFieldsFromResponse(hnsResponse *hcsshim.HNSEndpoint, ep *HNSEndpoint) {
	ep.ID = hnsResponse.Id
	ep.IPAddress = &net.IPNet{
		IP:   hnsResponse.IPAddress,
		Mask: net.CIDRMask(int(hnsResponse.PrefixLength), 32),
	}
	ep.MacAddress = hnsResponse.MacAddress
	ep.Gateway = hnsResponse.GatewayAddress
}
