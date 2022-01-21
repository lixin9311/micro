package utils

import (
	"os"
	"strconv"
	"strings"
)

const (
	envServiceType = "SERVICE_TYPE"
)

func GetDefaultPort(serviceType string, defaultPort int) int {
	if strings.EqualFold(os.Getenv(envServiceType), serviceType) {
		if portStr := os.Getenv("PORT"); portStr != "" {
			if port, err := strconv.Atoi(portStr); err != nil {
				return port
			}
		}
	}
	return defaultPort
}
