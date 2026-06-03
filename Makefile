all: clean ctsubmit

ctsubmit:
	CGO_ENABLED=0 go build -o $@ -ldflags "-X github.com/crtsh/ctsubmit/config.BuildTimestamp=`date --utc +%Y-%m-%dT%H:%M:%SZ`"

clean:
	rm -f ctsubmit