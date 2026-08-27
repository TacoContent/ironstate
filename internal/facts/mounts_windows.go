//go:build windows

package facts

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func init() {
	platformMounts = windowsMounts
}

// windowsMounts gathers two kinds of "mount": local volumes (drive
// letters and NTFS folder mount points, via the Win32 volume-enumeration
// API) and mapped network drives (via WNetGetConnection) — Windows has no
// single unified mount table like POSIX's /proc/mounts, so these are two
// separate passes.
func windowsMounts() ([]MountFact, error) {
	mounts, err := localVolumeMounts()
	if err != nil {
		return nil, err
	}
	netMounts, err := mappedDriveMounts()
	if err != nil {
		return mounts, err
	}
	return append(mounts, netMounts...), nil
}

// localVolumeMountSource labels every local-volume entry the same way,
// mirroring darwinMountSource — Windows has no fstab/mtab-equivalent file
// to name here either.
const localVolumeMountSource = "GetVolumePathNamesForVolumeName"

// localVolumeMounts enumerates every mounted volume via FindFirstVolume/
// FindNextVolume, then resolves each one's mount point(s) — usually a
// single drive letter, but a volume mounted into an NTFS folder (no drive
// letter) or into more than one place surfaces as one MountFact per path,
// same as Ansible's one-entry-per-mount-point convention.
func localVolumeMounts() ([]MountFact, error) {
	var volumeName [100]uint16
	handle, err := windows.FindFirstVolume(&volumeName[0], uint32(len(volumeName)))
	if err != nil {
		return nil, fmt.Errorf("FindFirstVolume: %w", err)
	}
	defer windows.FindVolumeClose(handle) //nolint:errcheck // best-effort cleanup, nothing actionable on failure

	var mounts []MountFact
	for {
		guidPath := windows.UTF16ToString(volumeName[:])
		mounts = append(mounts, volumeMountFacts(guidPath)...)

		err := windows.FindNextVolume(handle, &volumeName[0], uint32(len(volumeName)))
		if err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				break
			}
			return mounts, fmt.Errorf("FindNextVolume: %w", err)
		}
	}
	return mounts, nil
}

// volumeMountFacts resolves one volume GUID path's mount point(s) and
// filesystem info. A volume this process can't query (e.g. an empty
// CD/DVD drive, or a volume with no assigned mount point at all) is
// skipped rather than failing the whole gather.
func volumeMountFacts(guidPath string) []MountFact {
	paths, err := volumeMountPoints(guidPath)
	if err != nil || len(paths) == 0 {
		return nil
	}

	fstype, options, err := volumeInfo(guidPath)
	if err != nil {
		return nil
	}

	mounts := make([]MountFact, 0, len(paths))
	for _, path := range paths {
		mounts = append(mounts, MountFact{
			Source:  localVolumeMountSource,
			Device:  guidPath,
			Path:    path,
			FSType:  fstype,
			Options: options,
		})
	}
	return mounts
}

// volumeMountPoints wraps GetVolumePathNamesForVolumeName, growing the
// buffer once on ERROR_MORE_DATA and parsing its double-NUL-terminated
// MULTI_SZ result into a []string.
func volumeMountPoints(guidPath string) ([]string, error) {
	guidPathPtr, err := windows.UTF16PtrFromString(guidPath)
	if err != nil {
		return nil, err
	}

	bufLen := uint32(260)
	for {
		buf := make([]uint16, bufLen)
		var returnLen uint32
		err := windows.GetVolumePathNamesForVolumeName(guidPathPtr, &buf[0], bufLen, &returnLen)
		if err == nil {
			return parseMultiSZ(buf), nil
		}
		if err == windows.ERROR_MORE_DATA && returnLen > bufLen {
			bufLen = returnLen
			continue
		}
		return nil, err
	}
}

// parseMultiSZ splits a MULTI_SZ buffer (consecutive NUL-terminated
// strings, ending with an extra NUL) into its individual strings.
func parseMultiSZ(buf []uint16) []string {
	var out []string
	start := 0
	for i, c := range buf {
		if c != 0 {
			continue
		}
		if i > start {
			out = append(out, windows.UTF16ToString(buf[start:i]))
		}
		start = i + 1
		if i+1 >= len(buf) || buf[i+1] == 0 {
			break
		}
	}
	return out
}

// volumeInfo wraps GetVolumeInformation, returning the filesystem name and
// a synthesized "rw"/"ro"[,"compressed"] options string from its flags —
// Windows has no native mount-options string, so this mirrors the
// rw/ro-plus-flags shape darwinMountOptions already reports.
func volumeInfo(guidPath string) (fstype, options string, err error) {
	guidPathPtr, err := windows.UTF16PtrFromString(guidPath)
	if err != nil {
		return "", "", err
	}
	var fsNameBuf [32]uint16
	var flags uint32
	err = windows.GetVolumeInformation(guidPathPtr, nil, 0, nil, nil, &flags, &fsNameBuf[0], uint32(len(fsNameBuf)))
	if err != nil {
		return "", "", err
	}

	opts := []string{"rw"}
	if flags&windows.FILE_READ_ONLY_VOLUME != 0 {
		opts[0] = "ro"
	}
	if flags&windows.FILE_VOLUME_IS_COMPRESSED != 0 {
		opts = append(opts, "compressed")
	}
	return windows.UTF16ToString(fsNameBuf[:]), strings.Join(opts, ","), nil
}

// mprDLL/wNetGetConnectionProc back wNetGetConnection — golang.org/x/sys/
// windows has no WNetGetConnection wrapper (mpr.dll isn't one of the DLLs
// it binds), so it's declared here the same way the standard library's
// own os package resolves less-common Win32 APIs: NewLazySystemDLL against
// a proc name, resolved once and reused.
var (
	mprDLL                = windows.NewLazySystemDLL("mpr.dll")
	wNetGetConnectionProc = mprDLL.NewProc("WNetGetConnectionW")
)

const errorMoreData = 234 // ERROR_MORE_DATA, per WinError.h — ntstatus/errno tables in x/sys/windows only cover a subset of these under a Go name.

// wNetGetConnection resolves a mapped drive letter's remote UNC target via
// WNetGetConnectionW, growing the buffer once on ERROR_MORE_DATA.
func wNetGetConnection(localName string) (string, error) {
	localPtr, err := windows.UTF16PtrFromString(localName)
	if err != nil {
		return "", err
	}
	bufLen := uint32(260)
	for {
		buf := make([]uint16, bufLen)
		// Raw pointers into localPtr/buf/bufLen, all owned locally (the
		// first freshly allocated above, the other two just above this
		// call) - required by the LazyDLL/NewProc.Call syscall shape,
		// which takes uintptr args with no typed alternative.
		localArg := uintptr(unsafe.Pointer(localPtr)) //nolint:gosec // see comment above
		bufArg := uintptr(unsafe.Pointer(&buf[0]))    //nolint:gosec // see comment above
		bufLenArg := uintptr(unsafe.Pointer(&bufLen)) //nolint:gosec // see comment above
		ret, _, _ := wNetGetConnectionProc.Call(localArg, bufArg, bufLenArg)
		switch ret {
		case 0: // NO_ERROR
			return windows.UTF16ToString(buf), nil
		case errorMoreData:
			continue // bufLen was updated in place with the required size
		default:
			return "", fmt.Errorf("WNetGetConnectionW(%s): error %d", localName, ret)
		}
	}
}

// mappedDriveMounts enumerates every drive letter (via GetLogicalDrives)
// whose GetDriveType reports DRIVE_REMOTE, then resolves each one's UNC
// target — the "net use Z: \\server\share" style mapping local volume
// enumeration never sees, since a mapped drive isn't a Win32 "volume".
// fstype/options are left blank: querying them (e.g. GetVolumeInformation
// on the mapped root) would touch the same possibly-unresponsive server
// WNetGetConnection just did, which is exactly the risk GatherMounts'
// timeout exists to bound — not worth doubling down on per drive.
func mappedDriveMounts() ([]MountFact, error) {
	driveMask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("GetLogicalDrives: %w", err)
	}

	var mounts []MountFact
	for i := 0; i < 26; i++ {
		if driveMask&(1<<uint(i)) == 0 {
			continue
		}
		letter := string(rune('A' + i))
		root := letter + `:\`
		rootPtr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		if windows.GetDriveType(rootPtr) != windows.DRIVE_REMOTE {
			continue
		}
		remote, err := wNetGetConnection(letter + ":")
		if err != nil {
			continue
		}
		mounts = append(mounts, MountFact{
			Source: "WNetGetConnection",
			Device: remote,
			Path:   root,
		})
	}
	return mounts, nil
}
