package server

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"crypto_utils"
	. "db"
	. "types"
	. "utils"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var privateKey *rsa.PrivateKey
var publicKey *rsa.PublicKey
var shadow map[string][]byte
var db DB
var database = "test.db"
var user = &TableDef{
	Name:   "shadow",
	Types:  []uint32{TYPE_BYTES, TYPE_BYTES},
	Cols:   []string{"uid", "password"},
	PKeys:  1,
	Prefix: 0,
}
var kv = &TableDef{
	Name:   "key_value",
	Types:  []uint32{TYPE_BYTES, TYPE_BYTES},
	Cols:   []string{"key", "value"},
	PKeys:  1,
	Prefix: 0,
}

// server blinding table C->{publicKey, uid, time}
var sessionTable map[string]Blindentry
var name string
var kvstore map[string]interface{}
var Requests chan NetworkData
var Responses chan NetworkData
var serverState State
var sessionUID string

func init() {
	privateKey = crypto_utils.NewPrivateKey()
	publicKey = &privateKey.PublicKey
	publicKeyBytes := crypto_utils.PublicKeyToBytes(publicKey)
	if err := os.WriteFile("SERVER_PUBLICKEY", publicKeyBytes, 0666); err != nil {
		panic(err)
	}
	serverState = INIT
	name = uuid.NewString()
	kvstore = make(map[string]interface{})
	sessionTable = make(map[string]Blindentry)
	Requests = make(chan NetworkData)
	Responses = make(chan NetworkData)
	shadow = make(map[string][]byte)
	sessionUID = ""
	db.Path = database
	os.Remove(database)
	db.Open()
	// db.TableNew(TDEF_META)

	db.TableNew(user)
	db.TableNew(kv)
	go receiveThenSend()
}

func receiveThenSend() {
	defer close(Responses)

	for request := range Requests {
		Responses <- process(request)
	}
}

// Input: a byte array representing a request from a client.
// Deserializes the byte array into a request and performs
// the corresponding operation. Returns the serialized
// response. This method is invoked by the network.
func process(requestData NetworkData) NetworkData {
	if serverState == INIT {
		// A --> S: {K_AS}Ks,{uid,"LOGIN",K_A,r,{uid,"LOGIN",K_A,r}k_A}K_AS
		//          ||       ||
		//          ||       ||
		// A --> S: part1   ,part2
		// fmt.Println("----S: state:", serverState)
		if len(requestData.Payload) < 256 {
			serverState = INIT
			return failureMessage("")
		}
		part1 := requestData.Payload[:256]
		part2 := requestData.Payload[256:]
		K_AS, _ := crypto_utils.DecryptPK(part1, privateKey)
		message_part2, _ := crypto_utils.DecryptSK(part2, K_AS)

		var message_object Message
		err := json.Unmarshal(message_part2, &message_object)
		if err != nil {
			return failureMessage("")
		}
		signature := message_object.Values
		verify_message := message_object
		verify_message.Values = nil
		verify_message_byte, _ := json.Marshal(verify_message)
		// verify
		verifykey, _ := crypto_utils.BytesToPublicKey(verify_message.PublicKey)
		if crypto_utils.Verify(signature, crypto_utils.Hash(verify_message_byte), verifykey) {

			if verify_message.Status == "LOGIN" {
				return DoPhase2Login(verify_message, K_AS, requestData)
			} else if verify_message.Status == "REGISTER" {
				return DoPhase2Register(verify_message, K_AS, requestData)
			}
		} else {
			return failureMessage("")
		}

	} else if serverState == SESSION {
		var request Request
		var response Response
		requestData.Payload, _ = crypto_utils.DecryptSK(requestData.Payload, sessionTable[requestData.Name].SessionKey)
		response.Uid = sessionTable[requestData.Name].Uid
		json.Unmarshal(requestData.Payload, &request)
		doOp(&request, &response)
		responseBytes, _ := json.Marshal(response)
		responseBytes = crypto_utils.EncryptSK(responseBytes, sessionTable[requestData.Name].SessionKey)
		return NetworkData{Payload: responseBytes, Name: name}

	}
	return failureMessage("")
}
func DoPhase2Register(verify_message Message, K_AS []byte, requestData NetworkData) NetworkData {
	tod := crypto_utils.TodToBytes(crypto_utils.ReadClock())
	password := verify_message.Pass
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	_, exist := shadow[verify_message.Uid]
	if err != nil || exist {
		return failureMessage(verify_message.Uid)

	} else {
		rec := (&Record{}).AddStr("uid", []byte(verify_message.Uid)).AddStr("password", hashed)
		fmt.Println(rec)
		db.Insert("shadow", *rec)
		Assert(true)
		shadow[verify_message.Uid] = hashed

	}
	signature_message := Message{
		Uid:    verify_message.Uid,
		Time:   tod,
		Status: "OK",
		Values: nil,
	}
	signature_message_bytes, _ := json.Marshal(signature_message)
	signature_message_bytes = crypto_utils.Sign(signature_message_bytes, privateKey)
	response_message := Message{
		Uid:    verify_message.Uid,
		Time:   tod,
		Status: "OK",
		Values: signature_message_bytes,
	}
	response_message_bytes, _ := json.Marshal(response_message)
	return_message := crypto_utils.EncryptSK(response_message_bytes, K_AS)

	return NetworkData{Payload: return_message, Name: name}
}

func verifyPassword(uid string, password string) bool {
	_, ok := shadow[uid]
	search_rec := (&Record{}).AddStr("uid", []byte(uid))

	ok, _ = db.Get("shadow", search_rec)
	if !ok {
		return false
	}
	err := bcrypt.CompareHashAndPassword(search_rec.Get("password").Str, []byte(password))

	// err := bcrypt.CompareHashAndPassword([]byte(shadow[uid]), []byte(password))
	return err == nil
}

func DoPhase2Login(verify_message Message, K_AS []byte, requestData NetworkData) NetworkData {
	// fmt.Println("Verify success")

	if !verifyPassword(verify_message.Uid, verify_message.Pass) {
		return failureMessage("")
	}
	newEntry := Blindentry{
		Uid:        verify_message.Uid,
		Tod:        verify_message.Time,
		PublicKey:  verify_message.PublicKey,
		SessionKey: K_AS,
	}
	sessionTable[requestData.Name] = newEntry
	sessionUID = verify_message.Uid
	serverState = SESSION
	tod := crypto_utils.TodToBytes(crypto_utils.ReadClock())
	signature_message := Message{
		Uid:    verify_message.Uid,
		Time:   tod,
		Status: "OK",
		Values: nil,
	}
	signature_message_bytes, _ := json.Marshal(signature_message)
	signature_message_bytes = crypto_utils.Sign(signature_message_bytes, privateKey)
	response_message := Message{
		Uid:    verify_message.Uid,
		Time:   tod,
		Status: "OK",
		Values: signature_message_bytes,
	}
	response_message_bytes, _ := json.Marshal(response_message)

	return_message := crypto_utils.EncryptSK(response_message_bytes, sessionTable[requestData.Name].SessionKey)

	return NetworkData{Payload: return_message, Name: name}
}
func failureMessage(uid string) NetworkData {
	var response Response
	response.Status = FAIL
	response.Uid = uid
	responseBytes, _ := json.Marshal(response)
	return NetworkData{Payload: responseBytes, Name: name}
}

// Input: request from a client. Returns a response.
// Parses request and handles a switch statement to
// return the corresponding response to the request's
// operation.
func doOp(request *Request, response *Response) {
	response.Status = FAIL
	switch request.Op {
	case NOOP:
		// NOTHING
	case LOGIN:
		doLogin(request, response)
	case LOGOUT:
		doLogout(request, response)
	case CREATE:
		doCreate(request, response)
	case DELETE:
		doDelete(request, response)
	case COPY:
		doCopy(request, response)
	case READ:
		doReadVal(request, response)
	case WRITE:
		doWriteVal(request, response)
	case CHANGE_PASS:
		doChangePassword(request, response)
	case MODACL:
		doModacl(request, response)
	case REVACL:
		doRevacl(request, response)

	default:
		// struct already default initialized to
		// FAIL status
	}
}

func doModacl(request *Request, response *Response) {
	if Modacl(
		request.Uid,
		request.Key,
		map[string][]string{
			"readers":   request.Readers,
			"writers":   request.Writers,
			"copyfroms": request.Copyfroms,
			"copytos":   request.Copytos,
			"indirects": request.Indirects,
		},
	) {
		response.Status = OK
	}
}
func doRevacl(request *Request, response *Response) {
	lists := Revacl(request.Uid, request.Key)
	if lists != nil {
		response.Status = OK
		response.Readers = lists["readers"]
		response.Writers = lists["writers"]
		response.Copyfroms = lists["copyfroms"]
		response.Copytos = lists["copytos"]
		response.Indirects = lists["indirects"]
		response.R = lists["R"]
		response.W = lists["W"]
		response.C_src = lists["Csrc"]
		response.C_dst = lists["Cdst"]
	}
}

/*
*
It lets
the user in a current session with currently registered password old_pass update their
password to new_pass, as follows.
{“op”: “CHANGE_PASS”
,
“old_pass”: “GoBigRed2025”, “new_pass”:
“NewBuilding2025”}
A CHANGE_PASS operation fails if a user does not provide the correct currently registered
password as the value for old_pass. Therefore, a user must have performed LOGIN and be
executing within a session in order for a CHANGE_PASS operation to succeed.

*
*/
func doChangePassword(request *Request, response *Response) {
	new_pass := request.New_pass
	old_pass := request.Old_pass
	response.Status = FAIL
	if verifyPassword(request.Uid, old_pass) {
		hashed, err := bcrypt.GenerateFromPassword([]byte(new_pass), bcrypt.DefaultCost)
		if err == nil {
			shadow[request.Uid] = hashed
			response.Status = OK
		}
	}
	response.Uid = request.Uid

}

/** begin operation methods **/
// Input: user id uid. Returns a response.
// Begins session with user. If session already
// exists then status is FAIL.
func doLogin(request *Request, response *Response) {
	if sessionUID == "" {
		response.Uid = request.Uid
		response.Status = OK
	} else {
		response.Status = FAIL
		response.Uid = sessionUID
	}
}

// Ends current session. Returns a response.
// If no session exists then status is FAIL.
func doLogout(request *Request, response *Response) {
	if sessionUID != "" {
		//"The same user u ends the current session"
		response.Uid = sessionUID
		sessionUID = ""
		response.Status = OK
		serverState = INIT
	}

}

// Input: key k, value v, metaval m. Returns a response.
// Sets the value and metaval for key k in the
// key-value store to value v and metavalue m.
func doCreate(request *Request, response *Response) {
	if _, ok := kvstore[request.Key]; !ok {
		rec := (&Record{}).
			AddStr("key", []byte(request.Key))
		if val, ok := request.Val.(string); ok {
			rec.AddStr("value", []byte(val))
		} else {
			response.Status = FAIL
			return
		}
		db.Insert("key_value", *rec)
		kvstore[request.Key] = request.Val
		Create(
			request.Uid,
			request.Key,
			map[string][]string{
				"readers":   request.Readers,
				"writers":   request.Writers,
				"copyfroms": request.Copyfroms,
				"copytos":   request.Copytos,
				"indirects": request.Indirects,
			},
		)
		// __print_dac__()
		response.Status = OK
	}
}

// Input: key k. Returns a response. Deletes key from
// key-value store. If key does not exist then take no
// action.
func doDelete(request *Request, response *Response) {
	if _, ok := kvstore[request.Key]; ok {
		if isOwner(request.Uid, request.Key) {
			delete(kvstore, request.Key)
			rec := (&Record{}).AddStr("key", []byte(request.Key))
			db.Delete("key_value", *rec)
			DeleteKey(request.Uid, request.Key)
			response.Status = OK
		}
	}
}

// Input: key src_key, value dst_key. Returns a response.
// Change value in the key-value store associated with
// key dst_key to value associated with key src_key.
// If either key does not exist then status is FAIL.
func doCopy(request *Request, response *Response) {
	rec1 := (&Record{}).AddStr("key", []byte(request.Src_key))
	ok1, _ := db.Get("key_value", rec1)
	rec2 := (&Record{}).AddStr("key", []byte(request.Dst_key))
	ok2, _ := db.Get("key_value", rec2)
	if ok1 && slices.Contains(Csrc(request.Src_key), request.Uid) {
		if ok2 && slices.Contains(Cdst(request.Dst_key), request.Uid) {
			new := (&Record{}).AddStr("key", []byte(request.Dst_key))
			new.AddStr("value", rec1.Get("value").Str)
			db.Update("key_value", *new)
			// kvstore[request.Dst_key] = kvstore[request.Src_key]
			response.Status = OK
		}
	}
}

// Input: key k. Returns a response with the value
// associated with key. If key does not exist
// then status is FAIL.
func doReadVal(request *Request, response *Response) {
	rec := (&Record{}).AddStr("key", []byte(request.Key))
	ok, err := db.Get("key_value", rec)
	// v, ok := kvstore[request.Key];
	if err == nil && ok && slices.Contains(R(request.Key), request.Uid) {
		response.Val = string(rec.Get("value").Str)
		response.Status = OK
	}
}

// Input: key k and value v. Returns a response.
// Change value in the key-value store associated
// with key k to value v. If key does not exist
// then status is FAIL.
func doWriteVal(request *Request, response *Response) {
	rec := (&Record{}).AddStr("key", []byte(request.Key))
	ok, err := db.Get("key_value", rec)
	// _, ok := kvstore[request.Key];
	if err == nil && ok && slices.Contains(W(request.Key), request.Uid) {
		new := (&Record{}).AddStr("key", []byte(request.Key))
		if val, ok := request.Val.(string); ok {
			new.AddStr("value", []byte(val))
		} else {
			response.Status = FAIL
			return
		}
		// kvstore[request.Key] = request.Val
		db.Update("key_value", *new)
		response.Status = OK
	}
}
