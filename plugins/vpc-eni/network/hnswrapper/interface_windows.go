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
	"net"

	"github.com/Microsoft/hcsshim"
)

var (
	// HnsDefaultMinVersion is the minimum version of HNS on Windows instances supported by the plugin.
	HnsDefaultMinVersion = hcsshim.HNSVersion1803
)

// WindowsNetworkBuilder provides an interface for building the HNS networks and endpoints.
type WindowsNetworkBuilder interface {
	CheckHNSVersion(hnsMinVersion hcsshim.HNSVersion) error
	FindOrCreateHNSNetwork(nw *HNSNetwork) (bool, error)
	DeleteHNSNetwork(nw *HNSNetwork) error
	FindOrCreateHNSEndpoint(ep *HNSEndpoint) (bool, error)
	DeleteHNSEndpoint(ep *HNSEndpoint) error
	AttachEndpoint(ep *HNSEndpoint) error
	CreatePolicy(ep *HNSEndpoint, policy interface{}) error
}

// HNSNetwork is the configuration of the HNS network.
type HNSNetwork struct {
	Name               string
	Type               string
	NetworkAdapterName string
	AddressPrefix      string
	Gateway            string
}

// HNSEndpoint is the configuration of the HNS endpoint.
type HNSEndpoint struct {
	ID                 string
	Name               string
	VirtualNetworkName string
	DNSSuffix          string
	DNSServerList      string
	MacAddress         string
	IPAddress          *net.IPNet
	Gateway            string
	Policies           []json.RawMessage
	NetNS              HCSNamespace
	Container          AttachedContainer
}

// HCSNamespace is the configuration for using HCS namespaces.
type HCSNamespace struct {
	UseNamespace bool
	NetNSName    string
}

// AttachedContainer provides the information about the container to which the endpoint is being attached.
type AttachedContainer struct {
	ContainerID      string
	IsInfraContainer bool
	InfraContainerID string
}

// HnsRoutePolicy is an HNS route policy.
type HnsRoutePolicy struct {
	hcsshim.Policy
	DestinationPrefix string `json:"DestinationPrefix,omitempty"`
	NeedEncap         bool   `json:"NeedEncap,omitempty"`
}

// HnsOutboundNATPolicy is an HNS Outbound NAT policy.
type HnsOutboundNATPolicy hcsshim.OutboundNatPolicy
