package xds

import (
	"math/rand"
	"net"
	"sync"

	xdspb "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	corepb "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
)

// SocketAddress holds a corepb.SocketAddress.
type SocketAddress struct {
	*corepb.SocketAddress
}

// Address returns the address from s.
func (s *SocketAddress) Address() net.IP { return net.ParseIP(s.GetAddress()) }

// Port returns the port from s.
func (s *SocketAddress) Port() uint16 { return uint16(s.GetPortValue()) }

type assignment struct {
	mu  sync.RWMutex
	cla map[string]*xdspb.ClusterLoadAssignment
}

// NewAssignment returns a pointer to an assignment.
func NewAssignment() *assignment {
	return &assignment{cla: make(map[string]*xdspb.ClusterLoadAssignment)}
}

// SetClusterLoadAssignment sets the assignment for the cluster to cla.
func (a *assignment) SetClusterLoadAssignment(cluster string, cla *xdspb.ClusterLoadAssignment) {
	// If cla is nil we just found a cluster, check if we already know about it, or if we need to make a new entry.
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.cla[cluster]
	if !ok {
		a.cla[cluster] = cla
		return
	}
	if cla == nil {
		return
	}
	a.cla[cluster] = cla

}

// ClusterLoadAssignment returns the assignment for the cluster or nil if there is none.
func (a *assignment) ClusterLoadAssignment(cluster string) *xdspb.ClusterLoadAssignment {
	a.mu.RLock()
	cla, ok := a.cla[cluster]
	a.mu.RUnlock()
	if !ok {
		return nil
	}
	return cla
}

func (a *assignment) clusters() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	clusters := make([]string, len(a.cla))
	i := 0
	for k := range a.cla {
		clusters[i] = k
		i++
	}
	return clusters
}

// Select selects a endpoint from cluster load assignments, using weighted random selection. It only selects endpoints that are reporting healthy.
func (a *assignment) Select(cluster string, locality []Locality, ignore bool) (*SocketAddress, bool) {
	cla := a.ClusterLoadAssignment(cluster)
	if cla == nil {
		return nil, false
	}

	total := 0
	healthy := 0
	for _, ep := range cla.Endpoints {
		for _, lb := range ep.GetLbEndpoints() {
			if !ignore && lb.GetHealthStatus() != corepb.HealthStatus_HEALTHY {
				continue
			}
			total += int(lb.GetLoadBalancingWeight().GetValue())
			healthy++
		}
	}
	if healthy == 0 {
		return nil, true
	}

	if total == 0 {
		// all weights are 0, randomly select one of the endpoints.
		r := rand.Intn(healthy)
		i := 0
		for _, ep := range cla.Endpoints {
			for _, lb := range ep.GetLbEndpoints() {
				if !ignore && lb.GetHealthStatus() != corepb.HealthStatus_HEALTHY {
					continue
				}
				if r == i {
					return &SocketAddress{lb.GetEndpoint().GetAddress().GetSocketAddress()}, true
				}
				i++
			}
		}
		return nil, true
	}

	r := rand.Intn(total) + 1
	for _, ep := range cla.Endpoints {
		for _, lb := range ep.GetLbEndpoints() {
			if !ignore && lb.GetHealthStatus() != corepb.HealthStatus_HEALTHY {
				continue
			}
			r -= int(lb.GetLoadBalancingWeight().GetValue())
			if r <= 0 {
				return &SocketAddress{lb.GetEndpoint().GetAddress().GetSocketAddress()}, true
			}
		}
	}
	return nil, true
}

// All returns all healthy endpoints.
func (a *assignment) All(cluster string, locality []Locality, ignore bool) ([]*SocketAddress, bool) {
	cla := a.ClusterLoadAssignment(cluster)
	if cla == nil {
		return nil, false
	}

	sa := []*SocketAddress{}
	for _, ep := range cla.Endpoints {
		for _, lb := range ep.GetLbEndpoints() {
			if !ignore && lb.GetHealthStatus() != corepb.HealthStatus_HEALTHY {
				continue
			}
			sa = append(sa, &SocketAddress{lb.GetEndpoint().GetAddress().GetSocketAddress()})
		}
	}
	return sa, true
}
