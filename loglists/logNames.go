package loglists

import (
	"bytes"
	"crypto/sha256"

	"github.com/crtsh/ctloglists"
	"github.com/google/certificate-transparency-go/loglist3"
)

func useCrtshLogNames(logList *loglist3.LogList) {
	for _, operator := range logList.Operators {
		for _, log := range operator.Logs {
			useCrtshLogNameForRFC6962Log(log)
		}
		for _, tiledLog := range operator.TiledLogs {
			useCrtshLogNameForStaticLog(tiledLog)
		}
	}
}

func useCrtshLogNameForRFC6962Log(log *loglist3.Log) {
	for _, o := range ctloglists.CrtshV3All.Operators {
		for _, l := range o.Logs {
			if bytes.Equal(log.LogID, l.LogID) {
				log.Description = l.Description
				return
			}
		}
	}
}

func useCrtshLogNameForStaticLog(tiledLog *loglist3.TiledLog) {
	for _, o := range ctloglists.CrtshV3All.Operators {
		for _, tl := range o.TiledLogs {
			if bytes.Equal(tiledLog.LogID, tl.LogID) {
				tiledLog.Description = tl.Description
				return
			}
		}
	}
}

type extraLogEntry struct {
	Operator string
	LogName  string
}

// extraLogNames holds log names registered by other packages (e.g., mimic logs).
var extraLogNames = map[[sha256.Size]byte]extraLogEntry{}

// RegisterLogName registers a log ID to name mapping for use by GetLogName.
func RegisterLogName(keyID [sha256.Size]byte, operator, name string) {
	extraLogNames[keyID] = extraLogEntry{Operator: operator, LogName: name}
}

// GetLogName returns the operator name and log description for the given LogID, searching all known log lists.
func GetLogName(logID []byte) (string, string) {
	for _, logList := range []*loglist3.LogList{UsableTLSLogs, ActiveTLSLogs, TestTLSLogs, UsableBIMILogs} {
		if logList == nil {
			continue
		}
		for _, operator := range logList.Operators {
			for _, log := range operator.Logs {
				if bytes.Equal(log.LogID, logID) {
					return operator.Name, log.Description
				}
			}
			for _, tiledLog := range operator.TiledLogs {
				if bytes.Equal(tiledLog.LogID, logID) {
					return operator.Name, tiledLog.Description
				}
			}
		}
	}

	// Check registered extra log names (e.g., mimic logs).
	if len(logID) == sha256.Size {
		var keyID [sha256.Size]byte
		copy(keyID[:], logID)
		if entry, ok := extraLogNames[keyID]; ok {
			return entry.Operator, entry.LogName
		}
	}

	return "", ""
}
