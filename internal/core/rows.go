package core

import (
	"fmt"
	"math"
	"net"
	"scyllascope/internal/model"
	"strconv"
	"strings"
)

func Text(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
func Inet(v any) string {
	switch x := v.(type) {
	case net.IP:
		return x.String()
	case []byte:
		if len(x) == 4 || len(x) == 16 {
			return net.IP(x).String()
		}
		return ""
	case string:
		if ip := net.ParseIP(x); ip != nil {
			return ip.String()
		}
	}
	return ""
}
func LoadBytes(v any) (int64, bool) {
	if v == nil {
		return 0, false
	}
	s := Text(v)
	if n, e := strconv.ParseInt(s, 10, 64); e == nil {
		return n, n >= 0
	}
	f := strings.Fields(strings.ToUpper(s))
	if len(f) < 1 {
		return 0, false
	}
	n, e := strconv.ParseFloat(f[0], 64)
	if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0, false
	}
	mul := float64(1)
	if len(f) > 1 {
		switch f[1] {
		case "B":
		case "KB", "KIB":
			mul = 1024
		case "MB", "MIB":
			mul = 1024 * 1024
		case "GB", "GIB":
			mul = 1024 * 1024 * 1024
		case "TB", "TIB":
			mul = 1024 * 1024 * 1024 * 1024
		default:
			return 0, false
		}
	}
	if n*mul > math.MaxInt64 {
		return 0, false
	}
	return int64(n * mul), true
}
func ClusterRow(row map[string]any) (model.Node, error) {
	a := Inet(row["peer"])
	if a == "" {
		return model.Node{}, fmt.Errorf("system.cluster_status row has invalid peer")
	}
	up, ok := row["up"].(bool)
	if !ok {
		return model.Node{}, fmt.Errorf("system.cluster_status row has no boolean up")
	}
	status := strings.ToUpper(Text(row["status"]))
	switch status {
	case "UN":
		status = "NORMAL"
	case "UJ":
		status = "JOINING"
	case "UL":
		status = "LEAVING"
	case "UM":
		status = "MOVING"
	}
	if status == "" {
		return model.Node{}, fmt.Errorf("system.cluster_status row missing status")
	}
	id := Text(row["host_id"])
	if id == "" {
		id = a
	}
	load, known := LoadBytes(row["load"])
	return model.Node{ID: id, HostID: Text(row["host_id"]), Name: a, Address: a, DC: Text(row["dc"]), Rack: Text(row["rack"]), Status: status, Up: up, MetadataKnown: true, LoadBytes: load, LoadKnown: known}, nil
}
