package main

import (
	"bufio"
	"log"
	"net"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
)

// logEntry represents a single parsed Xray access log line.
type logEntry struct {
	srcIP   net.IP
	email   string
	inbound string
}

// logLineRe parses: "2026/03/28 10:41:22.571790 from 1.2.3.4:56789 accepted tcp:host:port [inbound-tag -> outbound] email: user@example.com"
var logLineRe = regexp.MustCompile(
	`from (\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}):\d+ accepted \S+ \[(\S+?) ->`,
)
var emailRe = regexp.MustCompile(`email: (\S+)$`)

// parseLogLine extracts srcIP, inbound tag, and email from one log line.
// Returns nil if the line is not a valid accepted connection line.
func parseLogLine(line string) *logEntry {
	m := logLineRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	ip := net.ParseIP(m[1])
	if ip == nil || ip.IsLoopback() {
		return nil // skip 127.0.0.1 (PROXY protocol not yet active or local)
	}
	em := emailRe.FindStringSubmatch(line)
	email := ""
	if em != nil {
		email = em[1]
	}
	return &logEntry{srcIP: ip, email: email, inbound: m[2]}
}

// GeoInfo holds resolved city/country for an IP.
type GeoInfo struct {
	Country string
	City    string
	ASN     string
}

// GeoCollector reads access.log and exposes per-user/per-inbound geo metrics.
type GeoCollector struct {
	mu      sync.RWMutex
	db      *geoip2.Reader
	asnDB   *geoip2.Reader
	logPath string

	// last seen geo per email: map[email]GeoInfo
	userGeo map[string]GeoInfo
	// connection count per (email, country, city): map[key]count
	userGeoConns map[string]int64
	// connection count per (inbound, country): map[key]count
	inboundGeoConns map[string]int64
}

func newGeoCollector(logPath, cityDBPath, asnDBPath string) *GeoCollector {
	db, err := geoip2.Open(cityDBPath)
	if err != nil {
		log.Printf("geo: cannot open city DB %s: %v (geo metrics disabled)", cityDBPath, err)
		return nil
	}
	var asnDB *geoip2.Reader
	if asnDBPath != "" {
		asnDB, err = geoip2.Open(asnDBPath)
		if err != nil {
			log.Printf("geo: cannot open ASN DB %s: %v (ASN label disabled)", asnDBPath, err)
		}
	}
	g := &GeoCollector{
		db:              db,
		asnDB:           asnDB,
		logPath:         logPath,
		userGeo:         make(map[string]GeoInfo),
		userGeoConns:    make(map[string]int64),
		inboundGeoConns: make(map[string]int64),
	}
	go g.tailLog()
	return g
}

// lookup resolves city and country for an IP.
func (g *GeoCollector) lookup(ip net.IP) GeoInfo {
	info := GeoInfo{Country: "unknown", City: "unknown"}
	if g.db == nil {
		return info
	}
	rec, err := g.db.City(ip)
	if err == nil {
		if rec.Country.IsoCode != "" {
			info.Country = rec.Country.IsoCode
		}
		if name, ok := rec.City.Names["en"]; ok && name != "" {
			info.City = name
		}
	}
	if g.asnDB != nil {
		asn, err := g.asnDB.ASN(ip)
		if err == nil && asn.AutonomousSystemOrganization != "" {
			info.ASN = asn.AutonomousSystemOrganization
		}
	}
	return info
}

// tailLog opens the log file and processes new lines continuously.
func (g *GeoCollector) tailLog() {
	for {
		if err := g.processLog(); err != nil {
			log.Printf("geo: log tail error: %v, retrying in 10s", err)
		}
		time.Sleep(10 * time.Second)
	}
}

func (g *GeoCollector) processLog() error {
	f, err := os.Open(g.logPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Seek to end to tail only new lines
	if _, err := f.Seek(0, 2); err != nil {
		return err
	}

	scanner := bufio.NewScanner(f)
	for {
		for scanner.Scan() {
			entry := parseLogLine(scanner.Text())
			if entry == nil {
				continue
			}
			geo := g.lookup(entry.srcIP)

			g.mu.Lock()
			if entry.email != "" {
				g.userGeo[entry.email] = geo
				key := entry.email + "\x00" + geo.Country + "\x00" + geo.City
				g.userGeoConns[key]++
			}
			ibKey := entry.inbound + "\x00" + geo.Country
			g.inboundGeoConns[ibKey]++
			g.mu.Unlock()
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		// No new lines yet — wait and retry
		time.Sleep(2 * time.Second)
		// Re-scan from current position (file may have been rotated)
		scanner = bufio.NewScanner(f)
	}
}

// snapshot returns copies of the geo maps for safe metric rendering.
func (g *GeoCollector) snapshot() (userGeo map[string]GeoInfo, userConns map[string]int64, ibConns map[string]int64) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	userGeo = make(map[string]GeoInfo, len(g.userGeo))
	for k, v := range g.userGeo {
		userGeo[k] = v
	}
	userConns = make(map[string]int64, len(g.userGeoConns))
	for k, v := range g.userGeoConns {
		userConns[k] = v
	}
	ibConns = make(map[string]int64, len(g.inboundGeoConns))
	for k, v := range g.inboundGeoConns {
		ibConns[k] = v
	}
	return
}
