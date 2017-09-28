// Copyright (c) 2015 Uber Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package mockhyperbahn

import (
	"fmt"
	"net"
	"strconv"

	hthrift "github.com/uber/tchannel-go/hyperbahn/gen-go/hyperbahn"
)

func toServicePeer(hostPort string) (*hthrift.ServicePeer, error) {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return nil, fmt.Errorf("invalid hostPort %v: %v", hostPort, err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("host %v is not an ip", host)
	}
	ip = ip.To4()

	if len(ip) != net.IPv4len {
		return nil, fmt.Errorf("ip %v is not a v4 ip, expected length to be %v, got %v",
			host, net.IPv4len, len(ip))
	}

	portInt, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("invalid port %v: %v", port, err)
	}

	// We have 4 bytes for the IP, use that as an int.
	ipInt := int32(uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3]))
	return &hthrift.ServicePeer{
		IP:   &hthrift.IpAddress{Ipv4: &ipInt},
		Port: int32(portInt),
	}, nil
}

func toServicePeers(hostPorts []string) ([]*hthrift.ServicePeer, error) {
	var peers []*hthrift.ServicePeer
	for _, hostPort := range hostPorts {
		peer, err := toServicePeer(hostPort)
		if err != nil {
			return nil, err
		}

		peers = append(peers, peer)
	}

	return peers, nil
}
