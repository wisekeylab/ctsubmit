package submitter

import json "github.com/goccy/go-json"

type LogType int

const (
	LOGTYPE_RFC6962 LogType = iota
	LOGTYPE_STATIC
)

func (logType LogType) MarshalJSON() ([]byte, error) {
	var s string
	switch logType {
	case LOGTYPE_RFC6962:
		s = "RFC6962"
	case LOGTYPE_STATIC:
		s = "STATIC"
	default:
		s = "UNKNOWN"
	}

	return json.Marshal(s)
}
