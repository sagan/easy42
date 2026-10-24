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

func TestParsePing_BusyBox(t *testing.T) {
	raw := `PING 172.20.0.53 (172.20.0.53): 56 data bytes
64 bytes from 172.20.0.53: seq=0 ttl=63 time=286.977 ms
64 bytes from 172.20.0.53: seq=1 ttl=63 time=286.209 ms

--- 172.20.0.53 ping statistics ---
2 packets transmitted, 2 packets received, 0% packet loss
round-trip min/avg/max = 286.209/286.593/286.977 ms
`
	p := ParsePing(raw)
	if p.Host != "172.20.0.53" || p.IP != "172.20.0.53" {
		t.Errorf("expected host/ip 172.20.0.53, got host=%s ip=%s", p.Host, p.IP)
	}
	if p.PacketsSent != 2 || p.PacketsReceived != 2 || p.PacketLossPct != 0.0 {
		t.Errorf("unexpected packet stats: sent=%d rec=%d loss=%f", p.PacketsSent, p.PacketsReceived, p.PacketLossPct)
	}
	if p.MinRTT != 286.209 || p.AvgRTT != 286.593 || p.MaxRTT != 286.977 {
		t.Errorf("unexpected RTTs: min=%f avg=%f max=%f", p.MinRTT, p.AvgRTT, p.MaxRTT)
	}
	if len(p.Packets) != 2 {
		t.Fatalf("expected 2 packet entries, got %d", len(p.Packets))
	}
	if p.Packets[0].Seq != 0 || p.Packets[0].TTL != 63 || p.Packets[0].TimeMs != 286.977 {
		t.Errorf("unexpected packet 0: %+v", p.Packets[0])
	}
	if p.Packets[1].Seq != 1 || p.Packets[1].TTL != 63 || p.Packets[1].TimeMs != 286.209 {
		t.Errorf("unexpected packet 1: %+v", p.Packets[1])
	}
}

func TestParsePing_IPv6(t *testing.T) {
	raw := `PING 2001:db8::1 (2001:db8::1): 56 data bytes
64 bytes from 2001:db8::1: seq=0 hlim=64 time=12.345 ms
64 bytes from 2001:db8::1: seq=1 hlim=64 time=12.100 ms

--- 2001:db8::1 ping statistics ---
2 packets transmitted, 2 packets received, 0% packet loss
round-trip min/avg/max = 12.100/12.222/12.345 ms
`
	p := ParsePing(raw)
	if p.Host != "2001:db8::1" || p.IP != "2001:db8::1" {
		t.Errorf("expected host/ip 2001:db8::1, got host=%s ip=%s", p.Host, p.IP)
	}
	if p.PacketsSent != 2 || p.PacketsReceived != 2 {
		t.Errorf("unexpected stats: sent=%d rec=%d", p.PacketsSent, p.PacketsReceived)
	}
	if len(p.Packets) != 2 {
		t.Fatalf("expected 2 packets, got %d", len(p.Packets))
	}
	if p.Packets[0].TTL != 64 || p.Packets[0].TimeMs != 12.345 {
		t.Errorf("unexpected pkt 0: %+v", p.Packets[0])
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

func TestParseTraceroute_NoSpaceAndNoDNS(t *testing.T) {
	raw := `traceroute to 172.20.0.53 (172.20.0.53), 30 hops max
  1   192.168.142.110  0.471ms  0.615ms  0.398ms 
  2   192.168.142.207  47.114ms  47.273ms  48.445ms 
  3   172.22.105.165  49.601ms  57.882ms  50.281ms 
  4   172.22.105.168  49.273ms  54.201ms  50.791ms 
  5   172.23.91.115  281.184ms  285.655ms  280.175ms 
  6   172.31.254.118  288.902ms  289.497ms  289.443ms 
  7   172.20.0.53  310.464ms  288.319ms  296.389ms 
`
	tr := ParseTraceroute(raw, "172.20.0.53")
	if len(tr.Hops) != 7 {
		t.Fatalf("expected 7 hops, got %d", len(tr.Hops))
	}
	for i, hop := range tr.Hops {
		if hop.TimedOut {
			t.Errorf("hop %d unexpectedly timed out", hop.Hop)
		}
		if len(hop.Times) != 3 {
			t.Errorf("hop %d expected 3 times, got %d (%v)", hop.Hop, len(hop.Times), hop.Times)
		}
		if i == 0 && hop.IP != "192.168.142.110" {
			t.Errorf("hop 1 expected IP 192.168.142.110, got %s", hop.IP)
		}
		if i == 6 && hop.IP != "172.20.0.53" {
			t.Errorf("hop 7 expected IP 172.20.0.53, got %s", hop.IP)
		}
	}
}

func TestParseTCPPing(t *testing.T) {
	raw := `{"target":"1.1.1.1:80","resolved_addr":"1.1.1.1:80","packets_sent":4,"packets_received":4,"packet_loss_pct":0.00,"min_rtt_ms":12.340,"avg_rtt_ms":14.500,"max_rtt_ms":16.780,"mdev_rtt_ms":1.620,"rtts":[12.340,14.500,16.780,14.380],"local_mss":1448,"remote_mss":1340}`
	parsed := ParseTCPPing(raw)
	if parsed == nil {
		t.Fatalf("expected non-nil result from ParseTCPPing")
	}
	if parsed.Target != "1.1.1.1:80" {
		t.Errorf("expected target 1.1.1.1:80, got %s", parsed.Target)
	}
	if parsed.PacketsSent != 4 || parsed.PacketsReceived != 4 {
		t.Errorf("unexpected packet counts: sent=%d, recv=%d", parsed.PacketsSent, parsed.PacketsReceived)
	}
	if len(parsed.RTTs) != 4 {
		t.Errorf("expected 4 RTT values, got %d", len(parsed.RTTs))
	}
	if parsed.LocalMSS != 1448 {
		t.Errorf("expected LocalMSS 1448, got %d", parsed.LocalMSS)
	}
	if parsed.RemoteMSS != 1340 {
		t.Errorf("expected RemoteMSS 1340, got %d", parsed.RemoteMSS)
	}

	// Test via ParseOutput
	res := ParseOutput("tcp_ping", raw, "1.1.1.1:80")
	tcpRes, ok := res.(*TCPPingResult)
	if !ok || tcpRes == nil {
		t.Fatalf("expected *TCPPingResult from ParseOutput")
	}
	if tcpRes.AvgRTT != 14.500 {
		t.Errorf("expected AvgRTT 14.500, got %f", tcpRes.AvgRTT)
	}
	if tcpRes.LocalMSS != 1448 || tcpRes.RemoteMSS != 1340 {
		t.Errorf("expected MSS (1448, 1340), got (%d, %d)", tcpRes.LocalMSS, tcpRes.RemoteMSS)
	}
}


