# pipe2log


Usage of pipe2log (check for updated documentation with pipe2log -help):
```
  -appname string
        what application name to use in syslog message. (default "bin/pipe2log")
  -cmd string
        currently can't be used for anything else than reading from pipe. (default "-")
  -facility string
        what syslog facility to use (default "local4").
        Valid options are: daemon, user, syslog, local[0-7]
  -hostname string
        what source/hostname to use in syslog message. (default "<the os hostname>")
        prefix the hostname with a plus sign "+" to combine it with the os hostname,
        +<my hostname>.<os hostname> useful for tracking docker container ids
  -logformat string
        default behaviour is to scan for severity, i.e. ERROR,DEBUG,CRIT,.. in
        the beginning of every line of input. Other options for logformat are
        'pm2json' and 'pino' for parsing NodeJs PM2/pino json output.
  -sysloguri string
        syslog host, i.e. localhost, /dev/log, (udp|tcp)://localhost[:514] (default "localhost")
        When using local log device /dev/log you can not change/set the hostname in the message.
        Local logging also implies rfc3164 format. Use 'console' for logging to stdout.
  -rfc3164
        format syslog messages using the rfc3164 protocol,
        default is to use the newer rfc5424 protocol.
  -rfc3339
        use rfc3339 timestamp in rfc3164 messages (has millisecond resolution).
  -version
        prints current app version
```

## Install

pre-compiled binaries for Mac OS (darwin) and Linux (debian) can be downloaded from https://github.com/issuu/pipe2log/releases, i.e.:

```
curl -L https://github.com/issuu/pipe2log/releases/download/<version tag>/pipe2log_{darwin,linux} \
     -o pipelog && chmod a+rx pipelog && ./pipelog -version
```

## Examples
```
<your program console output> 2>&1 | pipe2log -sysloguri logserver -logformat pm2json -appname myawesomeapp
```

## Mac OS

When testing on a Mac OS system, the default setting of the Mac syslog daemon is to only log severity warn, err, crit and alert. Output can be found in /var/log/system.log or in the console application.
