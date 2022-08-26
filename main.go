package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"runtime"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

const ROUTES = 15000

func getRoutes() (routes []netlink.Route, err error) {
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: unix.RT_TABLE_UNSPEC}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return routes, fmt.Errorf("failed to list routes: %v", err)
	}

	return routes, err
}

func repro() error {
	// Save the current network namespace
	fmt.Println("+ Saving current netns")
	origns, err := netns.Get()
	if err != nil {
		return fmt.Errorf("failed to get original ns: %v", err)
	}
	defer origns.Close()

	fmt.Println("+ Creating test netns")
	ns, err := netns.New()
	if err != nil {
		return fmt.Errorf("failed to create new netns: %v", err)
	}
	defer ns.Close()

	fmt.Println("+ Sanity check that initial route list is empty")
	routes, err := getRoutes()
	if err != nil {
		return err
	}
	if len(routes) != 0 {
		return fmt.Errorf("inital route list not empty")
	}

	fmt.Println("+ Grabbing handle to loopback interface")
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("failed to grab handle to lo: %v", err)
	}

	fmt.Println("+ Bringing lo up")
	if err := netlink.LinkSetUp(lo); err != nil {
		return fmt.Errorf("failed to bring lo up")
	}

	fmt.Printf("+ Adding %d fake routes\n", ROUTES)
	buf := make([]byte, 4)
	for i := 0; i < ROUTES; i++ {
		// Generate essentially random IP
		binary.LittleEndian.PutUint32(buf, uint32(i))
		ip := net.IP(buf)

		// Insert route
		if err := netlink.RouteAdd(&netlink.Route{
			LinkIndex: lo.Attrs().Index,
			Dst: &net.IPNet{
				IP:   ip,
				Mask: net.IPv4Mask(255, 255, 255, 255),
			},
		}); err != nil {
			return fmt.Errorf("failed to add route %d: %v", i, err)
		}
	}

	fmt.Println("+ Collecting memory stats before")
	before := runtime.MemStats{}
	runtime.ReadMemStats(&before)

	fmt.Println("+ Reading back routes")
	routes, err = getRoutes()
	if err != nil {
		return err
	}

	fmt.Println("+ Collecting memory stats after")
	after := runtime.MemStats{}
	runtime.ReadMemStats(&after)

	// Return to original ns
	fmt.Println("+ Returning to original netns")
	netns.Set(origns)

	fmt.Println("\n\n=============================")
	fmt.Printf("Found %d routes\n", len(routes))
	fmt.Printf("Memory delta: %dM\n", (after.HeapAlloc-before.HeapAlloc)>>20)

	return nil
}

func main() {
	// Lock the OS Thread so we don't accidentally switch namespaces
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := repro(); err != nil {
		panic(err)
	}
}
