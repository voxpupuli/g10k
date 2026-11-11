package g10k

import (
	"fmt"
	"time"
)

func (s *pfParserState) HandleForgeBaseURL(value string) error {
	s.puppetFile.forgeBaseURL = value
	return nil
}

func (s *pfParserState) HandleForgeCacheTTL(value string, line string) error {
	ttl, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("error: Can not convert value %s of parameter forge.cacheTtl %s to a golang Duration. valid time units are 300ms, 1.5h or 2h45m. In %s line: %s", value, value, s.pfPath, line)
	}
	s.puppetFile.forgeCacheTTL = ttl
	return nil
}
