package livestatus

import (
	"reflect"
	"testing"

	"github.com/oceanplexian/gogios/internal/objects"
)

func TestServiceDependencyColumnsExposeMasterServices(t *testing.T) {
	host := &objects.Host{Name: "node"}
	nrdc := &objects.Service{Host: host, Description: "nrdc"}
	nodeReady := &objects.Service{Host: host, Description: "K8s Node Ready"}
	kubelet := &objects.Service{
		Host:        host,
		Description: "K8s Kubelet Health",
		Dynamic:     true,
		NotifyDeps: []*objects.ServiceDependency{
			{Service: nrdc},
			{Service: nodeReady},
		},
	}
	table := servicesTable()
	want := []string{"node;nrdc", "node;K8s Node Ready"}
	for _, columnName := range []string{"depends_notify", "parents"} {
		got := table.Columns[columnName].Extract(kubelet)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", columnName, got, want)
		}
	}
	if got := table.Columns["is_dynamic"].Extract(kubelet); got != 1 {
		t.Errorf("is_dynamic = %#v, want 1", got)
	}
}

func TestHostDynamicColumn(t *testing.T) {
	host := &objects.Host{Name: "node", Dynamic: true}
	if got := hostsTable().Columns["is_dynamic"].Extract(host); got != 1 {
		t.Errorf("is_dynamic = %#v, want 1", got)
	}
}
