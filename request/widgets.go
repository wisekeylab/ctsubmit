package request

import (
	"fmt"
	"strings"
	"time"

	"github.com/crtsh/ctsubmit/config"
	"github.com/crtsh/ctsubmit/endpoint"
)

func copyright() string {
	var s strings.Builder

	copyrightFromYear := 2026
	s.WriteString(fmt.Sprintf(`<P class="copyright">&copy; <A href="https://sectigo.com" target="_blank">Sectigo</A> Limited %d`, copyrightFromYear))
	copyrightToYear := time.Now().Year()
	if copyrightToYear > copyrightFromYear {
		s.WriteString(fmt.Sprintf(`-%d`, copyrightToYear))
	}
	s.WriteString(`. All rights reserved.</P>`)

	return s.String()
}

func builtAt() string {
	return `<BR><B>Built at:</B><DIV style="font-size:8pt;font-style:normal">` + config.BuildTimestamp + `</DIV>`
}

func dashboards() string {
	return `
        <B>Dashboards:</B>
        <BR><BR><A href="/` + endpoint.ENDPOINTSTRING_DASHBOARD + `?loglist=usabletls">Usable TLS Logs</A>
        <BR><BR><A href="/` + endpoint.ENDPOINTSTRING_DASHBOARD + `?loglist=activetls">Active TLS Logs</A>
        <BR><BR><A href="/` + endpoint.ENDPOINTSTRING_DASHBOARD + `?loglist=testtls">Test TLS Logs</A>
        <BR><BR><A href="/` + endpoint.ENDPOINTSTRING_DASHBOARD + `?loglist=usablebimi">Usable BIMI Logs</A>`
}
