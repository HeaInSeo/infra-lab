package lab

import (
	"reflect"
	"testing"
)

func TestParseLibvirtBlockErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "none", in: "No errors found\n", want: nil},
		{name: "single", in: "vda: no space\n", want: []string{"vda: no space"}},
		{name: "multiple", in: "\nvda: no space\nvdb: read failed\n", want: []string{"vda: no space", "vdb: read failed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseLibvirtBlockErrors(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseLibvirtBlockErrors() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseLibvirtBlockPaths(t *testing.T) {
	in := ` Type   Device   Target   Source
------------------------------------------------
 file   disk     vda      /var/lib/libvirt/images/ebpf-dev.qcow2
 file   cdrom    sda      /var/lib/libvirt/images/ebpf-dev-seed.iso
 block  disk     vdb      -
`
	want := []string{
		"/var/lib/libvirt/images/ebpf-dev.qcow2",
		"/var/lib/libvirt/images/ebpf-dev-seed.iso",
	}
	if got := ParseLibvirtBlockPaths(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseLibvirtBlockPaths() = %#v, want %#v", got, want)
	}
}

func TestParseDFFreeKiB(t *testing.T) {
	in := `Filesystem 1024-blocks Used Available Capacity Mounted on
/dev/vda1 1000000 999000 1000 100% /var/lib/libvirt/images
`
	got, ok := ParseDFFreeKiB(in)
	if !ok {
		t.Fatal("ParseDFFreeKiB() ok=false")
	}
	if got != 1000 {
		t.Fatalf("ParseDFFreeKiB() = %d, want 1000", got)
	}
}

func TestDiagnoseLibvirtVMPaused(t *testing.T) {
	findings := DiagnoseLibvirtVM(VMInfo{Name: "vm1", Backend: "libvirt", State: "paused"})
	if len(findings) == 0 || findings[0].Code != "LIBVIRT_VM_PAUSED" {
		t.Fatalf("DiagnoseLibvirtVM() = %#v, want LIBVIRT_VM_PAUSED", findings)
	}
}
