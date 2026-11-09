// Package kvmcheck provides read-only diagnostics for a Linux KVM/libvirt host.
package kvmcheck

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Status describes a diagnostic outcome. Warnings do not block readiness.
type Status string

const (
	Pass    Status = "pass"
	Warn    Status = "warn"
	Fail    Status = "fail"
	Skipped Status = "skipped"
)

// Result is one diagnostic, with a stable ID suitable for UI rendering.
// Remediation is jsut advice for the user
type Result struct {
	ID          string `json:"id"`
	Status      Status `json:"status"`
	Message     string `json:"message"`
	Output      string `json:"output,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

// Report contains the check result
type Report struct {
	Ready  bool     `json:"ready"`
	Checks []Result `json:"checks"`
}

const commandTimeout = 5 * time.Second

type host struct {
	goos      string
	lookPath  func(string) (string, error)
	run       func(context.Context, string, ...string) (string, error)
	accessKVM func() error
	groups    func() ([]string, error)
	euid      func() int
}

// Check inspects the host.
// Each subprocess has a five-second timeout, further limited by ctx.
// Missing tools and host failures are returned as diagnostics and do not block other checks.
// Caller should set an overall context deadline.
func Check(ctx context.Context) Report {
	return check(ctx, host{
		goos: runtime.GOOS, lookPath: exec.LookPath, run: runCommand,
		accessKVM: accessKVM, groups: sessionGroups, euid: os.Geteuid,
	})
}

func check(ctx context.Context, h host) Report {
	report := Report{Ready: true, Checks: []Result{}}
	add := func(r Result) {
		report.Checks = append(report.Checks, r)
		if r.Status == Fail || r.Status == Skipped {
			report.Ready = false
		}
	}
	if h.goos != "linux" {
		add(Result{ID: "host.platform", Status: Fail, Message: "KVM host checks require Linux.", Output: h.goos, Remediation: "Run Tiny Machine Manager on a Linux KVM host."})
		return report
	}
	if err := ctx.Err(); err != nil {
		add(Result{ID: "check.context", Status: Fail, Message: "Host check canceled.", Output: err.Error(), Remediation: "Retry the host check with a live context."})
		return report
	}

	// Probe virsh first. Executables are checked rather than distro-specific
	// package databases so equivalent installations can satisfy requirements.
	tools := []struct{ name, packages string }{
		{"virsh", "libvirt-clients"},
		{"lsmod", "kmod"},
		{"kvm-ok", "cpu-checker"},
		{"virt-host-validate", "libvirt-clients (or libvirt-daemon-common, depending on distribution)"},
		{"qemu-system-x86_64", "qemu-system-x86"},
		{"qemu-img", "qemu-utils"},
		{"brctl", "bridge-utils"},
		{"virt-install", "virtinst"},
		{"systemctl", "systemd"},
	}
	available := make(map[string]bool, len(tools))
	for _, tool := range tools {
		path, err := h.lookPath(tool.name)
		r := Result{ID: "tool." + tool.name, Status: Pass, Message: tool.name + " is installed.", Output: path}
		if err != nil {
			r.Status, r.Message, r.Output = Fail, tool.name+" is unavailable in PATH.", err.Error()
			r.Remediation = "Install " + tool.packages + " using your distribution's package manager, then ensure " + tool.name + " is in PATH."
		} else {
			available[tool.name] = true
		}
		add(r)
	}
	run := func(name string, args ...string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		commandCtx, cancel := context.WithTimeout(ctx, commandTimeout)
		defer cancel()
		return h.run(commandCtx, name, args...)
	}
	commandCheck := func(id, name, message, fix string, args ...string) (Result, error) {
		r := Result{ID: id, Status: Pass, Message: message}
		if !available[name] {
			r.Status, r.Message, r.Remediation = Skipped, "Cannot check: "+name+" is unavailable.", "Install the missing tool and retry."
			return r, nil
		}
		output, err := run(name, args...)
		r.Output = strings.TrimSpace(output)
		if err != nil {
			r.Status, r.Message, r.Remediation = Fail, name+" check failed.", fix
			r.Output = diagnosticOutput(output, err)
		}
		return r, err
	}
	addCommand := func(id, name, message, fix string, args ...string) {
		result, _ := commandCheck(id, name, message, fix, args...)
		add(result)
	}

	modules, _ := commandCheck("kvm.modules", "lsmod", "KVM kernel modules are loaded.", "Enable hardware virtualization in BIOS/UEFI and load kvm plus kvm_intel or kvm_amd for your CPU.")
	if modules.Status == Pass {
		found := false
		for line := range strings.SplitSeq(modules.Output, "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == "kvm" {
				found = true
				break
			}
		}
		if !found {
			modules.Status, modules.Message = Fail, "No kvm module was reported by lsmod."
			modules.Remediation = "Enable hardware virtualization in BIOS/UEFI and load kvm plus kvm_intel or kvm_amd. A kernel with built-in KVM may omit it from lsmod; inspect that kernel's configuration."
		}
	}
	add(modules)

	device := Result{ID: "kvm.device", Status: Pass, Message: "The current user can open /dev/kvm for read/write."}
	if err := h.accessKVM(); err != nil {
		device.Status, device.Message, device.Output = Fail, "Cannot access /dev/kvm.", err.Error()
		device.Remediation = "Enable KVM and ensure /dev/kvm exists and is accessible to your session, usually through the kvm group. Sign out and back in after changing groups."
	}
	add(device)
	if h.euid() == 0 && available["kvm-ok"] {
		// Ubuntu's kvm-ok may execute modprobe msr as root. Never invoke it
		// with those privileges, even if /dev/kvm currently exists.
		add(Result{ID: "kvm.acceleration", Status: Warn, Message: "kvm-ok was not run as root because it can load kernel modules.", Remediation: "Run the app as your normal desktop user to include kvm-ok diagnostics. Device access and host validation are still checked."})
	} else {
		addCommand("kvm.acceleration", "kvm-ok", "kvm-ok confirms KVM acceleration.", "Review kvm-ok output; enable virtualization in BIOS/UEFI, load KVM modules and check /dev/kvm access.")
	}
	validation, validationErr := commandCheck("host.validation", "virt-host-validate", "QEMU host validation passed.", "Resolve the failures reported by virt-host-validate qemu.", "qemu")
	if validation.Status != Skipped {
		// Inspect the validator's status column, not words in its explanatory
		// text. Only a normal process exit with WARN output is downgraded.
		if validationFail.MatchString(validation.Output) {
			validation.Status, validation.Message = Fail, "QEMU host validation reported failures."
			validation.Remediation = "Resolve the failures reported by virt-host-validate qemu."
		} else if validationWarn.MatchString(validation.Output) {
			var exitErr *exec.ExitError
			if validationErr == nil || (errors.As(validationErr, &exitErr) && exitErr.ExitCode() != -1) {
				validation.Status, validation.Message = Warn, "QEMU host validation reported warnings."
				validation.Remediation = "Review the WARN lines; features such as PCI passthrough may need additional configuration."
			}
		}
	}
	add(validation)

	active := Result{ID: "libvirt.active", Status: Fail, Message: "No active libvirt service or socket was found.", Remediation: "Install libvirt-daemon-system. For libvirtd: sudo systemctl enable --now libvirtd. On modular installations, enable the virtqemud socket and any required driver sockets."}
	enabled := Result{ID: "libvirt.enabled", Status: Warn, Message: "Libvirt boot enablement was not confirmed.", Remediation: "Enable your libvirt service or socket at boot, for example: sudo systemctl enable --now libvirtd"}
	if !available["systemctl"] {
		active.Status, active.Message = Skipped, "Cannot check libvirt service state without systemctl."
		enabled.Status, enabled.Message = Skipped, "Cannot check libvirt boot enablement without systemctl."
	} else {
		var activity, boot []string
		for _, unit := range []string{"libvirtd.service", "libvirtd.socket", "virtqemud.service", "virtqemud.socket"} {
			output, err := run("systemctl", "is-active", unit)
			activity = append(activity, unit+": "+diagnosticOutput(output, err))
			if err == nil && strings.TrimSpace(output) == "active" {
				active.Status, active.Message, active.Remediation = Pass, unit+" is active.", ""
			}
			output, err = run("systemctl", "is-enabled", unit)
			boot = append(boot, unit+": "+diagnosticOutput(output, err))
			if err == nil && strings.TrimSpace(output) == "enabled" {
				enabled.Status, enabled.Message, enabled.Remediation = Pass, unit+" is enabled at boot.", ""
			}
		}
		active.Output, enabled.Output = strings.Join(activity, "\n"), strings.Join(boot, "\n")
	}
	add(active)
	add(enabled)

	groups := Result{ID: "user.groups", Status: Pass, Message: "The current session belongs to the libvirt and kvm groups."}
	if h.euid() == 0 {
		groups.Message = "Running as root; supplementary libvirt/kvm groups are not required."
	} else {
		names, err := h.groups()
		groups.Output = strings.Join(names, " ")
		membership := make(map[string]bool, len(names))
		for _, name := range names {
			membership[name] = true
		}
		if err != nil || !membership["libvirt"] || !membership["kvm"] {
			groups.Status, groups.Message = Fail, "Cannot confirm libvirt and kvm membership for the current session."
			groups.Output = diagnosticOutput(groups.Output, err)
			groups.Remediation = "Run sudo usermod -aG libvirt,kvm $USER, then sign out and back in and restart the app."
		}
	}
	add(groups)
	addCommand("libvirt.connection", "virsh", "Read-only connection to qemu:///system succeeded.", "Check libvirt service/socket state and current-session permissions for qemu:///system.", "--readonly", "--connect", "qemu:///system", "list", "--all")
	if err := ctx.Err(); err != nil {
		add(Result{ID: "check.context", Status: Fail, Message: "Host check canceled or timed out.", Output: err.Error(), Remediation: "Retry with a longer overall deadline if necessary."})
	}
	return report
}

var (
	validationFail = regexp.MustCompile(`(?m):[ \t]*FAIL(?:[ \t(]|$)`)
	validationWarn = regexp.MustCompile(`(?m):[ \t]*WARN(?:[ \t(]|$)`)
)

func diagnosticOutput(output string, err error) string {
	output = strings.TrimSpace(output)
	if err == nil {
		return output
	}
	if output == "" {
		return err.Error()
	}
	return output + "\n" + err.Error()
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	// Stable English output and no terminal control sequences from validators.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C", "TERM=dumb")
	// Bound waiting for inherited output pipes if a command spawns children.
	cmd.WaitDelay = 100 * time.Millisecond
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return string(output), err
}

func accessKVM() error {
	file, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return file.Close()
}

func sessionGroups() ([]string, error) {
	ids, err := os.Getgroups()
	if err != nil {
		return nil, err
	}
	ids = append(ids, os.Getegid())
	var names []string
	for _, id := range ids {
		group, err := user.LookupGroupId(strconv.Itoa(id))
		if err != nil {
			return names, fmt.Errorf("resolve session group %d: %w", id, err)
		}
		names = append(names, group.Name)
	}
	return names, nil
}
