package lookingglass

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"
)

var (
	// Ping regexes
	pingPacketRegex = regexp.MustCompile(`bytes from ([^:]+): icmp_seq=(\d+) ttl=(\d+) time=([\d\.]+) ms`)
	pingLossRegex   = regexp.MustCompile(`(\d+) packets transmitted, (\d+) received.*?[,\s]+([\d\.]+)% packet loss`)
	pingRTTRegex    = regexp.MustCompile(`(?:rtt|round-trip) min/avg/max/(?:mdev|stddev) = ([\d\.]+)/([\d\.]+)/([\d\.]+)/([\d\.]+) ms`)

	// Traceroute regexes
	traceHopRegex = regexp.MustCompile(`^\s*(\d+)\s+([^\s]+)\s+\(([\d\.:a-fA-F]+)\)(.*)$`)
	traceTimeRegex = regexp.MustCompile(`([\d\.]+)\s+ms`)

	// MTR report regex
	mtrRowRegex = regexp.MustCompile(`^\s*(\d+)\.\|--\s+([^\s]+)\s+([\d\.]+)%\s+(\d+)\s+([\d\.]+)\s+([\d\.]+)\s+([\d\.]+)\s+([\d\.]+)\s+([\d\.]+)`)
)

// ParseOutput dispatches raw command output to the designated parser
func ParseOutput(parser string, raw string, target string) interface{} {
	switch parser {
	case "bird_protocols":
		return ParseBIRDProtocols(raw)
	case "bird_route":
		return ParseBIRDRoute(raw, target)
	case "ping":
		return ParsePing(raw)
	case "traceroute":
		return ParseTraceroute(raw, target)
	case "mtr":
		return ParseMTR(raw, target)
	default:
		return nil
	}
}

// ParseBIRDProtocols parses `birdc show protocols` or `birdc show protocols all`
func ParseBIRDProtocols(raw string) []BirdProtocolEntry {
	var entries []BirdProtocolEntry
	scanner := bufio.NewScanner(strings.NewReader(raw))

	// Example header: Name       Proto      Table      State  Since         Info
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "BIRD") || strings.HasPrefix(line, "Name ") || strings.HasPrefix(line, "Access restricted") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// Check if first line starts with protocol name and type
		name := fields[0]
		proto := fields[1]
		table := fields[2]
		state := fields[3]
		since := ""
		info := ""

		if len(fields) >= 5 {
			since = fields[4]
		}
		if len(fields) >= 6 {
			info = strings.Join(fields[5:], " ")
		}

		connected := strings.EqualFold(state, "up") || strings.Contains(strings.ToLower(info), "established")

		entries = append(entries, BirdProtocolEntry{
			Name:      name,
			Proto:     proto,
			Table:     table,
			State:     state,
			Since:     since,
			Info:      info,
			Connected: connected,
		})
	}
	return entries
}

// ParseBIRDRoute parses `birdc show route for ... all` or `birdc show route`
func ParseBIRDRoute(raw string, target string) BirdRouteResult {
	var routes []BirdRouteEntry
	scanner := bufio.NewScanner(strings.NewReader(raw))

	var current *BirdRouteEntry
	currentNetwork := ""

	// Regex to match a route start line:
	// e.g.: "172.20.0.0/14        unicast [ibgp_node2 12:34:56.789] * (100/10) [AS4242421234i]"
	// or continuation route without network prefix:
	//       "                     unicast [ebgp_peer 11:22:33.444] (100/20) [AS4242429999 4242421234i]"
	routeHeaderRegex := regexp.MustCompile(`^(\S+)?\s+(unicast|blackhole|unreachable)\s+\[([^\]]+)\]\s*(\*)?\s*(\([^\)]+\))?\s*(\[[^\]]+\])?`)

	for scanner.Scan() {
		rawLine := scanner.Text()
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "BIRD") || strings.HasPrefix(line, "Table ") || strings.HasPrefix(line, "Access restricted") {
			continue
		}

		if matches := routeHeaderRegex.FindStringSubmatch(rawLine); len(matches) > 0 {
			if current != nil {
				routes = append(routes, *current)
				current = nil
			}

			netStr := strings.TrimSpace(matches[1])
			if netStr != "" {
				currentNetwork = netStr
			}

			protoInfo := strings.TrimSpace(matches[3])
			isBest := matches[4] == "*"
			metric := strings.Trim(strings.TrimSpace(matches[5]), "()")
			asPathRaw := strings.Trim(strings.TrimSpace(matches[6]), "[]")

			// Parse proto and time: "[ibgp_node2 12:34:56.789]"
			protoFields := strings.Fields(protoInfo)
			fromProto := ""
			since := ""
			if len(protoFields) > 0 {
				fromProto = protoFields[0]
				if len(protoFields) > 1 {
					since = strings.Join(protoFields[1:], " ")
				}
			}

			// Clean AS path: e.g. "AS4242421234i" -> ["4242421234"]
			var asPath []string
			if asPathRaw != "" {
				for _, part := range strings.Fields(asPathRaw) {
					clean := strings.TrimPrefix(part, "AS")
					clean = strings.TrimSuffix(clean, "i")
					clean = strings.TrimSuffix(clean, "?")
					if clean != "" {
						asPath = append(asPath, clean)
					}
				}
			}

			current = &BirdRouteEntry{
				Network:   currentNetwork,
				Best:      isBest,
				FromProto: fromProto,
				Since:     since,
				Metric:    metric,
				ASPath:    asPath,
			}
			continue
		}

		if current == nil {
			continue
		}

		// Detail lines
		if strings.HasPrefix(line, "via ") {
			// e.g. "via 10.42.0.2 on wg_node2"
			viaParts := strings.Split(strings.TrimPrefix(line, "via "), " on ")
			if len(viaParts) > 0 {
				current.NextHop = strings.TrimSpace(viaParts[0])
				current.Via = strings.TrimSpace(viaParts[0])
			}
			if len(viaParts) > 1 {
				current.Interface = strings.TrimSpace(viaParts[1])
			}
		} else if strings.HasPrefix(line, "BGP.as_path:") {
			rawPath := strings.TrimSpace(strings.TrimPrefix(line, "BGP.as_path:"))
			current.ASPath = strings.Fields(rawPath)
		} else if strings.HasPrefix(line, "BGP.next_hop:") {
			current.NextHop = strings.TrimSpace(strings.TrimPrefix(line, "BGP.next_hop:"))
		} else if strings.HasPrefix(line, "BGP.local_pref:") {
			current.LocalPref = strings.TrimSpace(strings.TrimPrefix(line, "BGP.local_pref:"))
		} else if strings.HasPrefix(line, "BGP.origin:") {
			current.Origin = strings.TrimSpace(strings.TrimPrefix(line, "BGP.origin:"))
		} else if strings.HasPrefix(line, "BGP.community:") {
			commStr := strings.TrimSpace(strings.TrimPrefix(line, "BGP.community:"))
			// Parse communities like "(64511, 1) (424242, 100)"
			re := regexp.MustCompile(`\([^\)]+\)`)
			current.Communities = re.FindAllString(commStr, -1)
		}
	}

	if current != nil {
		routes = append(routes, *current)
	}

	return BirdRouteResult{
		Target: target,
		Routes: routes,
	}
}

// ParsePing parses standard Linux ping output
func ParsePing(raw string) PingResult {
	res := PingResult{
		Packets: []PingPacket{},
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Target header: PING 1.1.1.1 (1.1.1.1) 56(84) bytes of data.
		if strings.HasPrefix(line, "PING ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				res.Host = fields[1]
			}
			if len(fields) >= 3 {
				res.IP = strings.Trim(fields[2], "()")
			}
			continue
		}

		// Packet line: 64 bytes from 1.1.1.1: icmp_seq=1 ttl=58 time=14.2 ms
		if match := pingPacketRegex.FindStringSubmatch(line); len(match) == 5 {
			host := match[1]
			seq, _ := strconv.Atoi(match[2])
			ttl, _ := strconv.Atoi(match[3])
			tMs, _ := strconv.ParseFloat(match[4], 64)

			res.Packets = append(res.Packets, PingPacket{
				Host:   host,
				Seq:    seq,
				TTL:    ttl,
				TimeMs: tMs,
			})
			continue
		}

		// Loss line: 4 packets transmitted, 4 received, 0% packet loss, time 3004ms
		if match := pingLossRegex.FindStringSubmatch(line); len(match) == 4 {
			res.PacketsSent, _ = strconv.Atoi(match[1])
			res.PacketsReceived, _ = strconv.Atoi(match[2])
			res.PacketLossPct, _ = strconv.ParseFloat(match[3], 64)
			continue
		}

		// RTT line: rtt min/avg/max/mdev = 13.921/14.187/14.512/0.218 ms
		if match := pingRTTRegex.FindStringSubmatch(line); len(match) == 5 {
			res.MinRTT, _ = strconv.ParseFloat(match[1], 64)
			res.AvgRTT, _ = strconv.ParseFloat(match[2], 64)
			res.MaxRTT, _ = strconv.ParseFloat(match[3], 64)
			res.MdevRTT, _ = strconv.ParseFloat(match[4], 64)
			continue
		}
	}

	return res
}

// ParseTraceroute parses standard traceroute output
func ParseTraceroute(raw string, target string) TracerouteResult {
	res := TracerouteResult{
		Target: target,
		Hops:   []TracerouteHop{},
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "traceroute to") {
			continue
		}

		// Check for timed out hop: e.g. " 3  * * *"
		timeoutRegex := regexp.MustCompile(`^\s*(\d+)\s+(\*\s*)+$`)
		if match := timeoutRegex.FindStringSubmatch(line); len(match) > 1 {
			hopNum, _ := strconv.Atoi(match[1])
			res.Hops = append(res.Hops, TracerouteHop{
				Hop:      hopNum,
				Host:     "*",
				TimedOut: true,
			})
			continue
		}

		// Standard hop: " 1  gateway (192.168.1.1)  0.345 ms  0.312 ms  0.289 ms"
		if match := traceHopRegex.FindStringSubmatch(line); len(match) == 5 {
			hopNum, _ := strconv.Atoi(match[1])
			host := match[2]
			ip := match[3]
			rest := match[4]

			var times []float64
			timeMatches := traceTimeRegex.FindAllStringSubmatch(rest, -1)
			for _, tm := range timeMatches {
				if len(tm) > 1 {
					val, _ := strconv.ParseFloat(tm[1], 64)
					times = append(times, val)
				}
			}

			res.Hops = append(res.Hops, TracerouteHop{
				Hop:      hopNum,
				Host:     host,
				IP:       ip,
				Times:    times,
				TimedOut: false,
			})
			continue
		}

		// Fallback for simple " 1  192.168.1.1  0.345 ms"
		simpleHopRegex := regexp.MustCompile(`^\s*(\d+)\s+([a-zA-Z0-9\.:-]+)\s+(.*)$`)
		if match := simpleHopRegex.FindStringSubmatch(line); len(match) == 4 {
			hopNum, _ := strconv.Atoi(match[1])
			hostOrIp := match[2]
			rest := match[3]

			var times []float64
			timeMatches := traceTimeRegex.FindAllStringSubmatch(rest, -1)
			for _, tm := range timeMatches {
				if len(tm) > 1 {
					val, _ := strconv.ParseFloat(tm[1], 64)
					times = append(times, val)
				}
			}

			res.Hops = append(res.Hops, TracerouteHop{
				Hop:      hopNum,
				Host:     hostOrIp,
				IP:       hostOrIp,
				Times:    times,
				TimedOut: len(times) == 0,
			})
		}
	}

	return res
}

// ParseMTR parses `mtr --report` table output
func ParseMTR(raw string, target string) MTRResult {
	res := MTRResult{
		Target: target,
		Hops:   []MTRHop{},
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if match := mtrRowRegex.FindStringSubmatch(line); len(match) == 10 {
			hop, _ := strconv.Atoi(match[1])
			host := match[2]
			loss, _ := strconv.ParseFloat(match[3], 64)
			sent, _ := strconv.Atoi(match[4])
			last, _ := strconv.ParseFloat(match[5], 64)
			avg, _ := strconv.ParseFloat(match[6], 64)
			best, _ := strconv.ParseFloat(match[7], 64)
			worst, _ := strconv.ParseFloat(match[8], 64)
			stdev, _ := strconv.ParseFloat(match[9], 64)

			res.Hops = append(res.Hops, MTRHop{
				Hop:     hop,
				Host:    host,
				LossPct: loss,
				Sent:    sent,
				Last:    last,
				Avg:     avg,
				Best:    best,
				Worst:   worst,
				StDev:   stdev,
			})
		}
	}

	return res
}
