package main

import (
    "bytes"
    "flag"
    "fmt"
    "regexp"
    "time"
    "bufio"
    "encoding/json"
    "log"
    //"log/syslog" // use the better and extended version of syslog
    syslog "github.com/issuu/srslog"
    "os"
    "os/exec"
    url "net/url"
    "go/scanner"
    "go/token"
    "strings"
)

const appTag = "pipe2log"

var (
    appVersion   = "1.0.0"
    appBuildTime = "2017-01-19 23:59:59 UTC"
    appGitHash   = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
)

var os_hostname string

// command line options / flags
var flagSyslogUri string
var flagSyslogFacility string
var flagSyslogTag string
var flagSyslogAppname string
var flagSyslogHostname string
var flagCommand string
var flagLogformat string
var flagVersion bool
var flagRFC3164 bool
var flagRFC3339 bool

// if using local log device we can't set/change hostname
var localLogging bool

var logWriter *syslog.Writer

type pm2Message struct {
    Message string      `json:"message"`
    Type string         `json:"type"`        // "out", "err", "process_event", ... ?
    Status string       `json:"status"`      // iff type is process_event
    App_name string     `json:"app_name"`
    Process_id int64    `json:"process_id"`
}

const RFC3339Milli = "2006-01-02T15:04:05.999Z07:00"
const RFC3339Micro = "2006-01-02T15:04:05.999999Z07:00"

// RFC5424Formatter provides an RFC 5424 compliant message.
// create our own customized version
func issuuRFC5424Formatter(p syslog.Priority, hostname, appname, content string) string {
    // SYSLOG-MSG      = HEADER SP STRUCTURED-DATA [SP MSG]
    // HEADER          = PRI VERSION SP TIMESTAMP SP HOSTNAME
    //                   SP APP-NAME SP PROCID SP MSGID
    // https://tools.ietf.org/html/rfc5424
    msgid := "-"            // syslog nil value
    structured_data := "-"  // syslog nil value
    timestamp := time.Now().Format(RFC3339Micro)
    pid := os.Getppid()
    if flagSyslogHostname != "" {
        if strings.HasPrefix(flagSyslogHostname,"+") {
            hostname = flagSyslogHostname[1:] + "." + os_hostname
        } else {
            hostname = flagSyslogHostname
        }
    }
    if hostname == "" {
        hostname = "-"  // syslog nil value
    }
    if appname == "" {
        appname = os.Args[0]
    }
    msg := fmt.Sprintf("<%d>%d %s %s %s %d %s %s %s",
        p, 1, timestamp, hostname, appname, pid, msgid, structured_data, content)
    return msg
}

// the original spec timestamp
const RFC3164 = "Jan 02 15:04:05"

// RFC3164ormatter provides an RFC 3164 message with RFC3339 timestamp.
// create our own customized version
func issuuRFC3164Formatter(p syslog.Priority, hostname, appname, content string) string {
    // SYSLOG-MSG      = PRI HEADER SP MSG
    // HEADER          = TIMESTAMP SP HOSTNAME_OR_IP
    // MSG             = TAG CONTENT
    // TIMESTAMP       = Mmm dd hh:mm:ss
    // https://tools.ietf.org/html/rfc3164
    var timestamp string
    if flagRFC3339 {
        timestamp = time.Now().Format(RFC3339Milli)
    } else {
        timestamp = time.Now().Format(RFC3164)
    }
    pid := os.Getppid()
    if flagSyslogHostname != "" {
        if strings.HasPrefix(flagSyslogHostname,"+") {
            hostname = flagSyslogHostname[1:] + "." + os_hostname
        } else {
            hostname = flagSyslogHostname
        }
    }
    if hostname == "" {
        hostname = "-"  // syslog nil value ? should be ip no
    }
    if appname == "" {
        appname = os.Args[0]
    }
    var msg string
    if localLogging {
        msg = fmt.Sprintf("<%d>%s %s[%d]: %s",
            p, timestamp, appname, pid, content)
    } else {
        msg = fmt.Sprintf("<%d>%s %s %s[%d]: %s",
            p, timestamp, hostname, appname, pid, content)
    }
    return msg
}

func checkError(err error) {
    if err != nil {
        log.Fatalf("Error: %s", err)
    }
}

type scandata struct {
  fdno int
  err error
  data []byte
}

func inputScanner(dc chan scandata, fdno int, s *bufio.Scanner) {
    defer close(dc)
    for s.Scan() {
        dc <- scandata{fdno: fdno, err: nil, data: s.Bytes()}
    }
    if err := s.Err(); err != nil {
        // not sure if s.Bytes() will contain anything on an error ?
        dc <- scandata{fdno: fdno, err: err, data: s.Bytes()}
    }
}

// ScanLines from the official go src - https://golang.org/src/bufio/scan.go
// I have included the code so I would be easier to improve on the parsing
// of log lines spread across multiple lines.
//
// ScanLines is a split function for a Scanner that returns each line of
// text, stripped of any trailing end-of-line marker. The returned line may
// be empty. The end-of-line marker is one optional carriage return followed
// by one mandatory newline. In regular expression notation, it is `\r?\n`.
// The last non-empty line of input will be returned even if it has no
// newline.
// dropCR drops a terminal \r from the data.
func dropCR(data []byte) []byte {
    if len(data) > 0 && data[len(data)-1] == '\r' {
        return data[0 : len(data)-1]
    }
    return data
}

func ScanLines(data []byte, atEOF bool) (advance int, scantoken []byte, err error) {
    if atEOF && len(data) == 0 {
        return 0, nil, nil
    }
    if i := bytes.IndexByte(data, '\n'); i >= 0 {
        // We have a full newline-terminated line.
        return i + 1, dropCR(data[0:i]), nil
    }
    // If we're at EOF, we have a final, non-terminated line. Return it.
    if atEOF {
        return len(data), dropCR(data), nil
    }
    // Request more data.
    return 0, nil, nil
}

// JSON Scanner Split Function - split on curly brackets
// the default go tokenizer handles quoted strings for us,
// so we do not to worry about curly brackets inside a string
//
func ScanJSON(data []byte, atEOF bool) (advance int, scantoken []byte, err error) {
    if atEOF && len(data) == 0 {
        return 0, nil, nil
    }

    // read open bracket
    if data[0] != '{' {
        if idx := bytes.IndexByte(data, '{'); idx >= 0 {
            log.Printf(appTag+" skipping up to next curly bracket: %s\n",data[0:idx])
            return idx, nil, nil
        }
        return len(data), nil, nil
    }

    var s scanner.Scanner
    fset := token.NewFileSet()                       // positions are relative to fset
    file := fset.AddFile("", fset.Base(), len(data)) // register input "file"
    s.Init(file, data, nil /* no error handler */, scanner.ScanComments)

    var braces int = 0
    var pos token.Pos
    var tok token.Token

    // the default go tokenizer handles quoted strings for us,
    // so we do not to worry about curly brackets inside a string
    for {
        switch pos, tok, _ = s.Scan(); tok {
        case token.EOF:
            // If we're at EOF, we have a final, non-terminated line. Return it.
            if atEOF {
                return len(data), data, nil
            }
            // Request more data.
            return 0, nil, nil
            //break loop
        case token.LBRACE:
            braces += 1
        case token.RBRACE:
            braces -= 1
            if (braces == 0) {
                // hooray we got an object
                p := file.Offset(pos)
                return p+2,data[0:p+1], nil
            }
        default:
            //fmt.Printf("%s\t%s\t%q\n", fset.Position(pos), tok, lit)
        }
    }

    // we should never reach this point

    // If we're at EOF, we have a final, non-terminated line. Return it.
    if atEOF {
        return len(data), data, nil
    }
    // Request more data.
    return 0, nil, nil
}

var severity_re = regexp.MustCompile("^[[]?((DEBUG|INFO|NOTICE|WARN|WARNING|ERR|ERROR|CRIT|CRITICAL|ALERT))[]]? (.*)$")

func processScanData(data scandata) {
    switch {
    case flagLogformat == "pm2json":
        var m pm2Message
        err := json.Unmarshal(data.data, &m)
        if err == nil {
            //fmt.Printf("decoded message: %s\n",m.Message)
            switch {
            case m.Type == "err":
                logWriter.Err(m.Message)
            case m.Type == "out":
                logWriter.Info(m.Message)
            case m.Type == "process_event":
                logmsg := fmt.Sprintf("%s: %s", m.Type, m.Status)
                logWriter.Debug(logmsg)
            default:
                logmsg := fmt.Sprintf("%s unknown pm2 log type '%s', data: '%s'", appTag, m.Type, data.data)
                logWriter.Crit(logmsg)
                log.Println(logmsg)
            }
        } else {
            logmsg := fmt.Sprintf("%s decoding error cannot parse json '%s', err '%s'", appTag, data.data, err)
            logWriter.Crit(logmsg)
            log.Println(logmsg)
        }
    default:
        rs := severity_re.FindSubmatch(data.data)
        if rs != nil {
            severity := fmt.Sprintf("%s",rs[2])
            msg := fmt.Sprintf("%s",rs[3])
            switch {
            case "DEBUG" == severity:
               logWriter.Debug(msg)
            case "INFO" == severity:
               logWriter.Info(msg)
            case "NOTICE" == severity:
               logWriter.Notice(msg)
            case "WARN" == severity || "WARNING" == severity:
               logWriter.Warning(msg)
            case "ERR" == severity || "ERROR" == severity:
               logWriter.Err(msg)
            case "CRIT" == severity || "CRITICAL" == severity:
               logWriter.Crit(msg)
            case "ALERT" == severity:
               logWriter.Alert(msg)
            default:
               // should never ever happen
               logmsg := fmt.Sprintf("%s unknown severity '%s' with msg '%s'", appTag, severity, msg)
               logWriter.Crit(logmsg)
               log.Fatalln(logmsg)
            }
        } else {
            // use default log severity ? a command option/flag ?
            msg := fmt.Sprintf("%s",data.data)
            logWriter.Info(msg)
        }
    }
}

func scanPipeLog() {
    var r1  *bufio.Scanner
    r1 = bufio.NewScanner(os.Stdin)

    if flagLogformat == "pm2json" {
        r1.Split(ScanJSON)
    } else {
        r1.Split(ScanLines)
    }

    dc1 := make(chan scandata, 1)
    go inputScanner(dc1, 1, r1)

    loop:for {
        select {
        case data, ok := <- dc1:
            if ok {
                processScanData(data)
                if (data.err != nil) {  break loop }
            } else {
                // channel closed
                break loop
            }
        default:
            time.Sleep(10 * time.Millisecond)
        }
    }
}

func scanCommand() {
    // NOT IMPLEMENTED YET
    // connect to cmds stdout and stderr channels
    var err error
    var r1, r2  *bufio.Scanner
    var cmd *exec.Cmd

    cmd = exec.Command(flagCommand, flag.Args()...)

    var w1, w2 *os.File
    var p1, p2 *os.File
    p1, w1, err = os.Pipe()
    checkError(err)
    p2, w2, err = os.Pipe()
    checkError(err)

    cmd.Stdout = w1
    cmd.Stderr = w2

    r1 = bufio.NewScanner(p1)
    r2 = bufio.NewScanner(p2)

    if flagLogformat == "pm2json" {
        r1.Split(ScanJSON)
        r2.Split(ScanJSON)
    } else {
        r1.Split(ScanLines)
        r2.Split(ScanLines)
    }

    err = cmd.Start()
    checkError(err)
    // Don't let main() exit before our command has finished running
    defer cmd.Wait()  // Doesn't block

    // now do some shit - loop and wait for data on r1 (stdout) and r2 (stderr)
    // also collect programs exit status and use this for exiting
}

func mapFacilityString(facility string) syslog.Priority {
    switch facility {
    case "daemon":
        return syslog.LOG_DAEMON
    case "user":
        return syslog.LOG_USER
    case "syslog":
        return syslog.LOG_SYSLOG
    case "local0":
        return syslog.LOG_LOCAL0
    case "local1":
        return syslog.LOG_LOCAL1
    case "local2":
        return syslog.LOG_LOCAL2
    case "local3":
        return syslog.LOG_LOCAL3
    case "local4":
        return syslog.LOG_LOCAL4
    case "local5":
        return syslog.LOG_LOCAL5
    case "local6":
        return syslog.LOG_LOCAL6
    case "local7":
        return syslog.LOG_LOCAL7
    default:
        log.Fatalf("Unsupported facility '%s', daemon, user, syslog, local[0-7] are supported.", facility);
        os.Exit(1)
    }
    // should never happen - but to keep go compiler happy
    return syslog.LOG_LOCAL4
}

func init() {
    var err error
    os_hostname, err = os.Hostname()
    if (err != nil) { os_hostname = "" }
    argv0 := os.Args[0]

    defaultSyslogUri      := "localhost"
    defaultSyslogFacility := "local4"
    defaultSyslogHostname := os_hostname
    defaultSyslogAppname  := argv0
    defaultCommand        := "-"
    defaultLogformat      := ""

    flag.BoolVar(&flagVersion, "version", false, "prints current app version")
    flag.BoolVar(&flagRFC3164, "rfc3164", false, "use original syslog rfc3164 msg format (default is to use rfc5424)")
    flag.BoolVar(&flagRFC3339, "rfc3339", false, "use rfc3339 timestamp (milliseconds) with rfc3164 message format")
    flag.StringVar(&flagSyslogUri, "sysloguri", defaultSyslogUri, "syslog host, i.e. localhost, /dev/log, (udp|tcp)://localhost[:514]. When using local log device /dev/log you can't change/set the hostname in the message. Local logging also implies rfc3164 format.")
    flag.StringVar(&flagSyslogFacility, "facility", defaultSyslogFacility, "what syslog facility to use.")
    flag.StringVar(&flagSyslogAppname, "appname", defaultSyslogAppname, "what application name to use in syslog message.")
    flag.StringVar(&flagSyslogHostname, "hostname", defaultSyslogHostname, "what source/hostname to use in syslog message, use a plus '+' prefix to combine the source with current existing hostname, useful for docker container ids.")
    flag.StringVar(&flagLogformat, "logformat", defaultLogformat, "default behaviour is to scan for severity, i.e. ERROR,DEBUG,CRIT,.. in the beginning of every line of input. Other options for logformat are 'pm2json' for parsing NodeJs PM2 json output.")
    flag.StringVar(&flagCommand, "cmd", defaultCommand, "currently can't be used for anything else than reading from pipe.")
}


func main() {

    var err error
    var u *url.URL

    flag.Parse()
    if flagVersion {
      fmt.Println(appVersion)
      fmt.Printf("Git commit hash: %s\n", appGitHash)
      fmt.Printf("UTC build time : %s\n", appBuildTime)
      os.Exit(0)
    }

    // other args: flag.Args() should be passed as cmd args

    // decode syslog_uri
    u, err = url.Parse(flagSyslogUri)
    // logserver:514 or just logserver
    if (u.Host == "" && (u.Path == "" && u.Scheme != "" || u.Path != "" && !strings.HasPrefix(u.Path,"/") && u.Scheme == "")) {
        u, err = url.Parse("udp://"+flagSyslogUri)
    }
    // host w/o port number
    if (u.Host != "" && strings.Index(u.Host,":") == -1) {
        u.Host += ":514"
    }
    if flagSyslogUri == "localhost" {
        u.Scheme = ""
        u.Host = ""
        u.Path = ""
    }
    // if using local log device we can't set/change hostname
    localLogging = u.Scheme == "" && u.Host == "" && strings.HasPrefix(u.Path,"/")

    if err != nil {
        log.Fatal(err)
    }

    syslog_facility := mapFacilityString(flagSyslogFacility)

    logWriter, err = syslog.Dial(u.Scheme, u.Host+u.Path, syslog.LOG_DEBUG|syslog_facility, flagSyslogAppname)
    checkError(err)

    // set syslog format
    if flagRFC3164 || localLogging {
        logWriter.SetFormatter(issuuRFC3164Formatter)
    } else {
        logWriter.SetFormatter(issuuRFC5424Formatter)
    }

    logWriter.Info(appTag+" program started.")

    // send some debug log - if running on a Mac anything not warning or worse are by default filtered out
    logWriter.Debug(appTag+" testing debug log statement.")
    logWriter.Info(appTag+" testing info log statement.")
    logWriter.Notice(appTag+" testing notice log statement.")
    logWriter.Warning(appTag+" testing warning log statement.")
    logWriter.Err(appTag+" testing error log statement.")
    logWriter.Crit(appTag+" testing critical log statement.")
    logWriter.Alert(appTag+" testing alert log statement.")

    if flagCommand == "-" {
        scanPipeLog()
    } else {
        scanCommand()
    }

    logWriter.Info(appTag+" program ended.")
    logWriter.Close()
}
