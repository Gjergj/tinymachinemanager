package kvmcheck

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func healthyHost(t *testing.T) host {
	t.Helper()
	return host{
		goos:      "linux",
		lookPath:  func(name string) (string, error) { return "/usr/bin/" + name, nil },
		accessKVM: func() error { return nil },
		groups:    func() ([]string, error) { return []string{"users", "kvm", "libvirt"}, nil },
		euid:      func() int { return 1000 },
		run: func(_ context.Context, name string, args ...string) (string, error) {
			switch name + " " + strings.Join(args, " ") {
			case "lsmod ":
				return "Module Size Used by\nkvm_intel 123 0\nkvm 456 1 kvm_intel\n", nil
			case "kvm-ok ":
				return "INFO: /dev/kvm exists\nKVM acceleration can be used", nil
			case "virt-host-validate qemu":
				return "QEMU: Checking for hardware virtualization : PASS\n", nil
			case "virsh --readonly --connect qemu:///system list --all":
				return " Id Name State\n----------------\n", nil
			case "systemctl is-active libvirtd.service":
				return "active\n", nil
			case "systemctl is-enabled libvirtd.service":
				return "enabled\n", nil
			default:
				if name == "systemctl" {
					return "inactive", errors.New("unit unavailable")
				}
				t.Fatalf("unexpected command: %s %v", name, args)
				return "", nil
			}
		},
	}
}

func resultByID(t *testing.T, report Report, id string) Result {
	t.Helper()
	for _, result := range report.Checks {
		if result.ID == id {
			return result
		}
	}
	t.Fatalf("missing result %q in %+v", id, report)
	return Result{}
}

func TestHealthyHost(t *testing.T) {
	h := healthyHost(t)
	lookup := h.lookPath
	var first string
	h.lookPath = func(name string) (string, error) {
		if first == "" {
			first = name
		}
		return lookup(name)
	}
	report := check(context.Background(), h)
	if !report.Ready {
		t.Fatalf("healthy host not ready: %+v", report)
	}
	if first != "virsh" {
		t.Fatalf("first executable checked = %q", first)
	}
	for _, id := range []string{"tool.virsh", "tool.qemu-system-x86_64", "tool.qemu-img", "tool.brctl", "tool.virt-install", "kvm.modules", "kvm.device", "kvm.acceleration", "host.validation", "libvirt.active", "libvirt.enabled", "user.groups", "libvirt.connection"} {
		if got := resultByID(t, report, id); got.Status != Pass {
			t.Errorf("%s = %+v", id, got)
		}
	}
	data, err := json.Marshal(report)
	if err != nil || !strings.Contains(string(data), `"ready":true`) {
		t.Fatalf("JSON report: %s, %v", data, err)
	}
}

func TestHostFailures(t *testing.T) {
	for _, tt := range []struct {
		name, id string
		change   func(*host)
	}{
		{"modules absent", "kvm.modules", func(h *host) { override(h, "lsmod", nil, "Module Size Used by\nvboxdrv 123 0", nil) }},
		{"module command failure", "kvm.modules", func(h *host) { override(h, "lsmod", nil, "permission denied", errors.New("exit 1")) }},
		{"device denied", "kvm.device", func(h *host) { h.accessKVM = func() error { return os.ErrPermission } }},
		{"device missing", "kvm.device", func(h *host) { h.accessKVM = func() error { return os.ErrNotExist } }},
		{"acceleration unavailable", "kvm.acceleration", func(h *host) { override(h, "kvm-ok", nil, "KVM acceleration cannot be used", errors.New("exit 1")) }},
		{"validator fails", "host.validation", func(h *host) {
			override(h, "virt-host-validate", []string{"qemu"}, "QEMU: Checking hardware : FAIL (disabled in BIOS)", &exec.ExitError{})
		}},
		{"validator launch fails", "host.validation", func(h *host) { override(h, "virt-host-validate", []string{"qemu"}, "", os.ErrPermission) }},
		{"validator timeout after warning", "host.validation", func(h *host) {
			override(h, "virt-host-validate", []string{"qemu"}, "QEMU: Checking IOMMU : WARN (optional)", context.DeadlineExceeded)
		}},
		{"validator mixed results", "host.validation", func(h *host) {
			override(h, "virt-host-validate", []string{"qemu"}, "QEMU: Checking IOMMU : WARN (optional)\nQEMU: Checking hardware : FAIL (disabled)", exitFailure(t))
		}},
		{"group missing", "user.groups", func(h *host) { h.groups = func() ([]string, error) { return []string{"kvm"}, nil } }},
		{"group lookup fails", "user.groups", func(h *host) { h.groups = func() ([]string, error) { return nil, errors.New("lookup failed") } }},
		{"connection denied", "libvirt.connection", func(h *host) {
			override(h, "virsh", []string{"--readonly", "--connect", "qemu:///system", "list", "--all"}, "authentication failed", errors.New("exit 1"))
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := healthyHost(t)
			tt.change(&h)
			report := check(context.Background(), h)
			result := resultByID(t, report, tt.id)
			if report.Ready || result.Status != Fail || result.Remediation == "" {
				t.Fatalf("unexpected report: %+v, result: %+v", report, result)
			}
		})
	}
}

func override(h *host, name string, args []string, output string, err error) {
	run := h.run
	h.run = func(ctx context.Context, got string, gotArgs ...string) (string, error) {
		if got == name && strings.Join(gotArgs, "\x00") == strings.Join(args, "\x00") {
			return output, err
		}
		return run(ctx, got, gotArgs...)
	}
}

func TestMissingTools(t *testing.T) {
	for _, tool := range []string{"virsh", "lsmod", "kvm-ok", "virt-host-validate", "qemu-system-x86_64", "qemu-img", "brctl", "virt-install", "systemctl"} {
		t.Run(tool, func(t *testing.T) {
			h := healthyHost(t)
			lookup, run := h.lookPath, h.run
			h.lookPath = func(name string) (string, error) {
				if name == tool {
					return "", exec.ErrNotFound
				}
				return lookup(name)
			}
			h.run = func(ctx context.Context, name string, args ...string) (string, error) {
				if name == tool {
					t.Fatalf("ran missing tool %s", tool)
				}
				return run(ctx, name, args...)
			}
			report := check(context.Background(), h)
			if report.Ready || resultByID(t, report, "tool."+tool).Status != Fail {
				t.Fatalf("missing tool accepted: %+v", report)
			}
			if resultByID(t, report, "kvm.device").Status != Pass {
				t.Fatal("independent check not run")
			}
			if tool == "virsh" && resultByID(t, report, "libvirt.connection").Status != Skipped {
				t.Fatal("dependent check not skipped")
			}
		})
	}
}

func TestValidatorWarnings(t *testing.T) {
	for _, commandErr := range []error{nil, exitFailure(t)} {
		h := healthyHost(t)
		override(&h, "virt-host-validate", []string{"qemu"}, "QEMU: Checking IOMMU support : WARN (optional for PCI passthrough)\n", commandErr)
		report := check(context.Background(), h)
		if !report.Ready || resultByID(t, report, "host.validation").Status != Warn {
			t.Fatalf("warnings block readiness: %+v", report)
		}
	}
}

func exitFailure(t *testing.T) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestExitFailureHelper$")
	cmd.Env = append(os.Environ(), "KVM_CHECK_EXIT=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("could not produce exit status 1: %v", err)
	}
	return err
}

func TestExitFailureHelper(t *testing.T) {
	if os.Getenv("KVM_CHECK_EXIT") == "1" {
		os.Exit(1)
	}
}

func TestLibvirtServiceStates(t *testing.T) {
	for _, tt := range []struct {
		name, unit, enabled     string
		wantActive, wantEnabled Status
		ready                   bool
	}{
		{"modular socket", "virtqemud.socket", "enabled", Pass, Pass, true},
		{"legacy socket", "libvirtd.socket", "enabled", Pass, Pass, true},
		{"modular daemon", "virtqemud.service", "enabled", Pass, Pass, true},
		{"disabled at boot", "libvirtd.service", "disabled", Pass, Warn, true},
		{"runtime enablement", "libvirtd.service", "enabled-runtime", Pass, Warn, true},
		{"inactive", "", "disabled", Fail, Warn, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := healthyHost(t)
			run := h.run
			h.run = func(ctx context.Context, name string, args ...string) (string, error) {
				if name != "systemctl" {
					return run(ctx, name, args...)
				}
				if len(args) != 2 {
					t.Fatalf("unexpected systemctl arguments: %v", args)
				}
				if args[1] == tt.unit {
					if args[0] == "is-active" {
						return "active", nil
					}
					if args[0] == "is-enabled" {
						return tt.enabled, nil
					}
				}
				return "inactive", errors.New("exit 3")
			}
			report := check(context.Background(), h)
			if report.Ready != tt.ready || resultByID(t, report, "libvirt.active").Status != tt.wantActive || resultByID(t, report, "libvirt.enabled").Status != tt.wantEnabled {
				t.Fatalf("unexpected service report: %+v", report)
			}
		})
	}
}

func TestRootDoesNotRequireSupplementaryGroups(t *testing.T) {
	h := healthyHost(t)
	h.euid = func() int { return 0 }
	h.groups = func() ([]string, error) { t.Fatal("root group lookup unnecessary"); return nil, nil }
	if report := check(context.Background(), h); !report.Ready {
		t.Fatalf("root rejected: %+v", report)
	}
}

func TestRootDoesNotRunPotentiallyMutatingKVMOK(t *testing.T) {
	h := healthyHost(t)
	h.euid = func() int { return 0 }
	run := h.run
	h.run = func(ctx context.Context, name string, args ...string) (string, error) {
		if name == "kvm-ok" {
			t.Fatal("kvm-ok can load the msr module as root")
		}
		return run(ctx, name, args...)
	}
	report := check(context.Background(), h)
	if !report.Ready || resultByID(t, report, "kvm.acceleration").Status != Warn {
		t.Fatalf("unexpected root report: %+v", report)
	}
}

func TestUnsupportedHostAndCancellation(t *testing.T) {
	for _, unsupported := range []bool{true, false} {
		h := healthyHost(t)
		ctx, cancel := context.WithCancel(context.Background())
		if unsupported {
			h.goos = "darwin"
		} else {
			cancel()
		}
		defer cancel()
		h.run = func(context.Context, string, ...string) (string, error) {
			t.Fatal("command should not run")
			return "", nil
		}
		report := check(ctx, h)
		if report.Ready {
			t.Fatalf("invalid host ready: %+v", report)
		}
	}
}

func TestCommandDeadlinesAndMidCheckCancellation(t *testing.T) {
	h := healthyHost(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := h.run
	calls := 0
	h.run = func(commandCtx context.Context, name string, args ...string) (string, error) {
		deadline, ok := commandCtx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("missing five-second command deadline")
		}
		calls++
		output, err := run(commandCtx, name, args...)
		cancel()
		return output, err
	}
	report := check(ctx, h)
	if report.Ready || calls != 1 || resultByID(t, report, "check.context").Status != Fail {
		t.Fatalf("cancellation ignored: calls=%d, report=%+v", calls, report)
	}
}

func TestCommandRunner(t *testing.T) {
	if os.Getenv("KVM_CHECK_HELPER") == "1" {
		if os.Getenv("KVM_CHECK_BLOCK") == "1" {
			time.Sleep(10 * time.Second)
		}
		if os.Getenv("LC_ALL") != "C" {
			os.Exit(2)
		}
		os.Stdout.WriteString("stdout\n")
		os.Stderr.WriteString("stderr\n")
		os.Exit(0)
	}
	t.Setenv("KVM_CHECK_HELPER", "1")
	output, err := runCommand(context.Background(), os.Args[0], "-test.run=^TestCommandRunner$")
	if err != nil || !reflect.DeepEqual(strings.Fields(output), []string{"stdout", "stderr"}) {
		t.Fatalf("output = %q, err = %v", output, err)
	}
	t.Setenv("KVM_CHECK_BLOCK", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = runCommand(ctx, os.Args[0], "-test.run=^TestCommandRunner$")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
}
