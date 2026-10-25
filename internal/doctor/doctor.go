package doctor

import (
	"context"
	"fmt"
	"net"
	"os"
	"scyllascope/internal/model"
	"scyllascope/internal/probe"
	"scyllascope/internal/settings"
	"strconv"
	"strings"
	"time"
)

type Result struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func Run(c *settings.Config) []Result {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	v := []Result{}
	add := func(name string, check model.Check) { v = append(v, Result{name, check.OK, check.Message}) }
	if _, e := os.Stat(c.Scylla.ConfigPath); e != nil {
		v = append(v, Result{"Scylla config", false, e.Error()})
	} else {
		v = append(v, Result{"Scylla config", true, c.Scylla.ConfigPath})
	}
	add("scylla-server", probe.Service(ctx, c.Scylla.SystemdService, c.Monitoring.Timeout))
	for _, h := range c.Scylla.CQLHosts {
		add("CQL TCP "+net.JoinHostPort(h, strconv.Itoa(c.Scylla.CQLPort)), probe.TCP(ctx, h, c.Scylla.CQLPort, c.Monitoring.Timeout))
		break
	}
	for _, p := range c.Peers {
		add("Internode TCP "+p.Address, probe.TCP(ctx, p.Address, c.Scylla.InternodePort, c.Monitoring.Timeout))
	}
	add("Local REST", probe.HTTP(ctx, probe.Client(c.Monitoring.Timeout), strings.TrimRight(c.Scylla.RESTURL, "/")+"/storage_service/host_id"))
	v = append(v, Result{"UI authentication", c.Security.UIToken != "", "systemd install generates /etc/scyllascope/ui-token"}, Result{"Peer authentication", c.Token != "", "same cluster_token required on all observers"})
	return v
}
func Print(v []Result) bool {
	all := true
	for _, r := range v {
		mark := "OK"
		if !r.OK {
			mark = "!!"
			all = false
		}
		fmt.Printf("%-3s %-30s %s\n", mark, r.Name, r.Message)
	}
	return all
}
