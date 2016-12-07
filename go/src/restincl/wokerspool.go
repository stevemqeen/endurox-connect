package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"ubftab"

	atmi "github.com/endurox-dev/endurox-go"
)

/*

So we will have a pool of goroutines which will wait on channel. This is needed
for doing less works with ATMI initialization and uninit.

Scheme will be following

- there will be array of goroutines, number is set in M_workwers
- there will be number same number of channels M_waitjobchan[M_workers]
- there will be M_freechan which will identify the free channel number (
when worker will complete its work, it will submit it's number to this channel)

So handler on new message will do <-M_freechan and then send message to -> M_waitjobchan[M_workers]
Workes will wait on <-M_waitjobchan[M_workers], when complete they will do Nr -> M_freechan

*/

//Needed for channel work submit
type HttpCall struct {
	w         http.ResponseWriter
	req       *http.Request
	terminate bool //Sent packet when thread should terminate
}

var M_freechan chan int //List of free channels submitted by wokers

var M_waitjobchan []chan HttpCall //Wokers channels each worker by it's number have a channel

//Generate response in the service configured way...
//@w	handler for writting response to
func GenRsp(ac *atmi.ATMICtx, buf atmi.TypedBuffer, svc *ServiceMap,
	w http.ResponseWriter, atmiErr atmi.ATMIError) {

	var rsp []byte
	var err atmi.ATMIError
	/*	application/json */
	rsp_type := "text/plain"

	//Have a common error handler
	if nil == atmiErr {
		err = atmi.NewCustomATMIError(atmi.TPMINVAL, "SUCCEED")
	} else {
		err = atmiErr
	}
	//Generate resposne accordingly...
	switch svc.Conv_int {
	case CONV_JSON2UBF:
		rsp_type = "text/json"
		//Convert buffer back to JSON & send it back..
		//But we could append the buffer with error here...

		bufu, ok := buf.(*atmi.TypedUBF)

		if !ok {
			ac.TpLogError("Failed to cast TypedBuffer to TypedUBF!")
			//Create empty buffer for generating response...

			if svc.Errors_int == ERRORS_JSONUBF {
				bufu, _ = ac.NewUBF(1024)
			}

		}

		//Add error & msg fields, if needed
		if svc.Errors_int == ERRORS_JSONUBF {
			bufu.BChg(ubftab.EX_IF_ECODE, 0, err.Code())
			bufu.BChg(ubftab.EX_IF_EMSG, 0, err.Message())
		}

		//Generate the resposne buffer...

		ret, err1 := bufu.TpUBFToJSON()

		if nil == err1 {
			rsp = []byte(ret)
		} else {
			rsp = []byte(fmt.Sprintf("{EX_IF_ECODE:%d, EX_IF_EMSG:\"%s\"}",
				err1.Code(), err1.Message()))
		}

		break
	case CONV_TEXT: //This is string buffer...
		//If there is no error & it is sync call, then just plot
		//a buffer back
		if 0 == svc.Asynccall && atmi.TPMINVAL == err.Code() {

			bufs, ok := buf.(*atmi.TypedString)

			if !ok {
				ac.TpLogError("Failed to cast buffer to TypedString")
				err = atmi.NewCustomATMIError(atmi.TPEINVAL,
					"Failed to cast buffer to TypedString")
			} else {
				//Set the bytes to string we got
				rsp = []byte(bufs.GetString())
			}
		}

		break
	case CONV_RAW: //This is carray..
		rsp_type = "application/octet-stream"
		if 0 == svc.Asynccall && atmi.TPMINVAL == err.Code() {

			bufs, ok := buf.(*atmi.TypedCarray)

			if !ok {
				ac.TpLogError("Failed to cast buffer to TypedCarray")
				err = atmi.NewCustomATMIError(atmi.TPEINVAL,
					"Failed to cast buffer to TypedCarray")
			} else {
				//Set the bytes to string we got
				rsp = bufs.GetBytes()
			}
		}
		break
	case CONV_JSON:
		rsp_type = "text/json"
		if 0 == svc.Asynccall && atmi.TPMINVAL == err.Code() {

			bufs, ok := buf.(*atmi.TypedJSON)

			if !ok {
				ac.TpLogError("Failed to cast buffer to TypedJSON")
				err = atmi.NewCustomATMIError(atmi.TPEINVAL,
					"Failed to cast buffer to ypedJSON")
			} else {
				//Set the bytes to string we got
				rsp = []byte(bufs.GetJSON())
			}
		}
		break
	}

	//OK Now if all ok, there is stuff in buffer (from JSONUBF) it will
	//be there in any case, thus we do not handle that

	switch svc.Errors_int {
	case ERRORS_HTTP:
		var lookup map[string]int
		//Map the resposne codes
		if len(svc.Errors_fmt_http_map) > 0 {
			lookup = svc.Errors_fmt_http_map
		} else {
			lookup = M_defaults.Errors_fmt_http_map
		}

		estr := strconv.Itoa(err.Code())

		http_code := 500

		if 0 != lookup[estr] {
			http_code = lookup[estr]
		} else {
			http_code = lookup["*"]
		}

		//Generate error response and pop out of the funcion
		if 200 != http_code {
			ac.TpLogWarn("Mapped response: tp %d -> http %d",
				err.Code(), http_code)
			w.WriteHeader(http_code)
		}

		break
	case ERRORS_JSON:
		//Send JSON error block, togher with buffer, if buffer empty
		//Send simple json...
		strrsp := string(rsp)
		if i := strings.LastIndex(strrsp, "}"); i > -1 {
			//Add the trailing response code in JSON block
			substring := strrsp[0:i]
			errs := fmt.Sprintf("%s, %s}",
				fmt.Sprintf(svc.Errfmt_json_code, err.Code()),
				fmt.Sprintf(svc.Errfmt_json_code, err.Message()))

			ac.TpLogWarn("Error code generated: [%s]", errs)
			strrsp = substring + errs

			ac.TpLogDebug("JSON Response generated: [%s]", strrsp)
		} else {
			rsp_type = "text/json"
			//Send plaint json
			strrsp = fmt.Sprintf("{%s, %s}",
				fmt.Sprintf(svc.Errfmt_json_code, err.Code()),
				fmt.Sprintf(svc.Errfmt_json_msg, err.Message()))
			ac.TpLogDebug("JSON Response generated (2): [%s]", strrsp)
		}

		rsp = []byte(strrsp)
		break
	case ERRORS_TEXT:
		//Send plain text
		rsp_type = "text/json"
		//Send plaint json
		strrsp := fmt.Sprintf(svc.Errfmt_text, err.Code(), err.Message())
		ac.TpLogDebug("TEXT Response generated (2): [%s]", strrsp)
		rsp = []byte(strrsp)

		break
	}

	//Send resposne back (if ok...)
	w.Header().Set("Content-Type", rsp_type)
	w.Write(rsp)
}

// Requesst handler
//@param ac	ATMI Context
//@param w	Response writer (as usual)
//@param req	Request message (as usual)
func HandleMessage(ac *atmi.ATMICtx, svc *ServiceMap, w http.ResponseWriter, req *http.Request) int {

	var flags int64 = 0
	var buf atmi.TypedBuffer
	var err atmi.ATMIError

	ac.TpLog(atmi.LOG_DEBUG, "Got URL [%s]", req.URL)
	/* Send json to service */
	//svc := M_url_map[req.URL.String()]

	if "" != svc.Svc {

		body, _ := ioutil.ReadAll(req.Body)

		ac.TpLogDebug("Requesting service [%s] buffer [%s]", svc.Svc, body)

		//Prepare outgoing buffer...
		switch svc.Conv_int {
		case CONV_JSON2UBF:
			//Convert JSON 2 UBF...

			bufu, err1 := ac.NewUBF(1024)

			if nil != err1 {
				ac.TpLogError("failed to alloca ubf buffer %d:[%s]\n",
					err1.Code(), err1.Message())
				return atmi.FAIL
			}

			ac.TpLogDebug("Converting to UBF: [%s]", string(body))

			if err1 := bufu.TpJSONToUBF(string(body)); err1 != nil {
				ac.TpLogError("Failed to conver buffer to JSON %d:[%s]\n",
					err1.Code(), err1.Message())

				ac.TpLogError("Failed req: [%s]", string(body))
				return atmi.FAIL
			}

			buf = bufu
			break
		case CONV_TEXT:
			//Use request buffer as string

			bufs, err1 := ac.NewString(string(body))

			if nil != err1 {
				ac.TpLogError("failed to alloc string/text buffer %d:[%s]\n",
					err1.Code(), err1.Message())
				return atmi.FAIL
			}

			buf = bufs

			break
		case CONV_RAW:
			//Use request buffer as binary

			bufc, err1 := ac.NewCarray(body)

			if nil != err1 {
				ac.TpLogError("failed to alloc carray/bin buffer %d:[%s]\n",
					err1.Code(), err1.Message())
				return atmi.FAIL
			}

			buf = bufc

			break
		case CONV_JSON:
			//Use request buffer as JSON

			bufj, err1 := ac.NewJSON(body)

			if nil != err1 {
				ac.TpLogError("failed to alloc carray/bin buffer %d:[%s]\n",
					err1.Code(), err1.Message())
				return atmi.FAIL
			}

			buf = bufj

			break
		}

		if err != nil {
			ac.TpLogError("ATMI Error %d:[%s]\n", err.Code(), err.Message())
			return atmi.FAIL
		}

		if svc.Notime {
			ac.TpLogWarn("No timeout flag for service call")
			flags |= atmi.TPNOTIME
		}

		//TODO: We need a support for starting global transaction
		if 0 != svc.Asynccall {
			//TODO: Add error handling.
			//In case if we have UBF, then we can send the same buffer
			//back, but append the response with error fields.
			_, err := ac.TpACall(svc.Svc, buf.GetBuf(), flags|atmi.TPNOREPLY)

			GenRsp(ac, buf, svc, w, err)

			/*
					flags|atmi.TPNOREPLY); err != nil {
					w.Header().Set("Content-Type", "text/plain")
					w.Write([]byte(err.Message()))
				} else {
					w.Header().Set("Content-Type", "text/json")
					w.Write([]byte(buf.GetJSONText()))
				}
			*/

		} else {
			_, err := ac.TpCall(svc.Svc, buf.GetBuf(), flags)
			GenRsp(ac, buf, svc, w, err)

			/*
				if _, err := ac.TpCall(svc.svc, buf.GetBuf(), flags); err != nil {
					w.Header().Set("Content-Type", "text/plain")
					w.Write([]byte(err.Message()))
				} else {
					w.Header().Set("Content-Type", "text/json")
					w.Write([]byte(buf.GetJSONText()))
				}
			*/
		}
	}

	return atmi.SUCCEED
}

//Run the worker
//@param mynr	Woker number
func WorkerRun(mynr int) {
	terminate := false
	//Get the ATMI context
	ac, err := atmi.NewATMICtx()

	if nil != err {
		fmt.Fprintf(os.Stderr, "Goroutine %d Failed to allocate cotnext: %s!", mynr, err)
		os.Exit(atmi.FAIL)
	}

	err = ac.TpInit()

	if nil != err {
		ac.TpLogError("Goroutine %d failed to TpInit!", mynr, err)
		os.Exit(atmi.FAIL)
	}

	//Run until we get terminate message
	for !terminate {

		ac.TpLogDebug("Goroutine %d is free, waiting for next job", mynr)
		M_freechan <- mynr

		workblock := <-M_waitjobchan[mynr]

		svc := M_url_map[workblock.req.URL.String()]

		//TODO: Open request file if needed
		if svc.Reqlogsvc != "" {
			//Get the request buffer... (early?)
			//ac.TpLogSetReqFile(data, filename, filesvc)
		}

		if !workblock.terminate {
			HandleMessage(ac, &svc, workblock.w, workblock.req)
		} else {
			terminate = true
			ac.TpLogWarn("Thread %d got terminate message", mynr)
		}

		//TODO: Close request file if was open...

	}
}

//Initialise channels and work pools
func InitPool() {

	M_freechan = make(chan int)

	for i := 0; i < M_workers; i++ {
		var callHanlder chan HttpCall
		M_waitjobchan = append(M_waitjobchan, callHanlder)
		go WorkerRun(i)
	}
}