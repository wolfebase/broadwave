//go:build !unix

package httpapi

func openFDs() int { return -1 }

func maxRSS() uint64 { return 0 }
