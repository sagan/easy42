package lookingglass

import (
	"testing"
)

func TestParseBIRDProtocols(t *testing.T) {
	raw := `BIRD 2.0.8 ready.
Name       Proto      Table      State  Since         Info
device1    Device     master4    up     2026-09-01    
direct1    Direct     master4    up     2026-09-01    
kernel1    Kernel     master4    up     2026-09-01    
ibgp_node2 BGP        ---        up     12:34:56.789  Established   
ebgp_dn42  BGP        ---        start  12:34:56.789  Active        Socket: Connection refused
`
	entries := ParseBIRDProtocols(raw)
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	if entries[3].Name != "ibgp_node2" || !entries[3].Connected || entries[3].State != "up" {
		t.Errorf("unexpected entry 3: %+v", entries[3])
	}

	if entries[4].Name != "ebgp_dn42" || entries[4].Connected || entries[4].State != "start" {
		t.Errorf("unexpected entry 4: %+v", entries[4])
	}
}

func TestParseBIRDRoute(t *testing.T) {
	raw := `BIRD 2.0.8 ready.
Table master4:
172.20.0.0/14        unicast [ibgp_node2 12:34:56.789] * (100/10) [AS4242421234i]
	via 10.42.0.2 on wg_node2
	Type: BGP univ
	BGP.origin: IGP
	BGP.as_path: 4242421234
	BGP.next_hop: 10.42.0.2
	BGP.local_pref: 100
	BGP.community: (64511, 1) (424242, 100)
                     unicast [ebgp_peer 11:22:33.444] (100/20) [AS4242429999 4242421234i]
	via 10.42.1.1 on wg_peer
	Type: BGP univ
	BGP.origin: IGP
	BGP.as_path: 4242429999 4242421234
	BGP.next_hop: 10.42.1.1
`
	res := ParseBIRDRoute(raw, "172.20.0.0/14")
	if len(res.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(res.Routes))
	}

	r1 := res.Routes[0]
	if !r1.Best || r1.FromProto != "ibgp_node2" || r1.NextHop != "10.42.0.2" {
		t.Errorf("unexpected route 1: %+v", r1)
	}
	if len(r1.Communities) != 2 {
		t.Errorf("expected 2 communities in route 1, got %v", r1.Communities)
	}

	r2 := res.Routes[1]
	if r2.Best || r2.FromProto != "ebgp_peer" {
		t.Errorf("unexpected route 2: %+v", r2)
	}
}

func TestParsePing(t *testing.T) {
	raw := `PING 1.1.1.1 (1.1.1.1) 56(84) bytes of data.
64 bytes from 1.1.1.1: icmp_seq=1 ttl=58 time=14.2 ms
64 bytes from 1.1.1.1: icmp_seq=2 ttl=58 time=14.5 ms

--- 1.1.1.1 ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 1002ms
rtt min/avg/max/mdev = 14.200/14.350/14.500/0.150 ms
`
	p := ParsePing(raw)
	if p.Host != "1.1.1.1" || p.IP != "1.1.1.1" {
		t.Errorf("expected host 1.1.1.1, got %s / %s", p.Host, p.IP)
	}
	if p.PacketsSent != 2 || p.PacketsReceived != 2 || p.PacketLossPct != 0.0 {
		t.Errorf("unexpected packet stats: sent=%d rec=%d loss=%f", p.PacketsSent, p.PacketsReceived, p.PacketLossPct)
	}
	if p.MinRTT != 14.200 || p.AvgRTT != 14.350 {
		t.Errorf("unexpected RTTs: min=%f avg=%f", p.MinRTT, p.AvgRTT)
	}
	if len(p.Packets) != 2 {
		t.Errorf("expected 2 packet entries, got %d", len(p.Packets))
	}
}

func TestParseTraceroute(t *testing.T) {
	raw := `traceroute to 1.1.1.1 (1.1.1.1), 30 hops max, 60 byte packets
 1  gateway (192.168.1.1)  0.345 ms  0.312 ms  0.289 ms
 2  * * *
 3  one.one.one.one (1.1.1.1)  14.210 ms  14.195 ms
`
	tr := ParseTraceroute(raw, "1.1.1.1")
	if len(tr.Hops) != 3 {
		t.Fatalf("expected 3 hops, got %d", len(tr.Hops))
	}
	if tr.Hops[0].Hop != 1 || tr.Hops[0].IP != "192.168.1.1" || len(tr.Hops[0].Times) != 3 {
		t.Errorf("unexpected hop 1: %+v", tr.Hops[0])
	}
	if tr.Hops[1].Hop != 2 || !tr.Hops[1].TimedOut {
		t.Errorf("unexpected hop 2: %+v", tr.Hops[1])
	}
	if tr.Hops[2].Hop != 3 || tr.Hops[2].IP != "1.1.1.1" {
		t.Errorf("unexpected hop 3: %+v", tr.Hops[2])
	}
}
