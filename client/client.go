package client

import (
	"crypto/rsa"
	"crypto_utils"
	"encoding/json"
	"os"
	. "types"

	"github.com/google/uuid"
)

var name string //client uid, not user uid
var Requests chan NetworkData
var Responses chan NetworkData

var serverPublicKey *rsa.PublicKey

var sessionUID string
var privteKey *rsa.PrivateKey
var sessionKey []byte

func init() {
	name = uuid.NewString()
	Requests = make(chan NetworkData)
	Responses = make(chan NetworkData)
	sessionUID = ""
	privteKey = crypto_utils.NewPrivateKey()
}

func ObtainServerPublicKey() {
	serverPublicKeyBytes, err := os.ReadFile("SERVER_PUBLICKEY")
	if err != nil {
		panic(err)
	}
	serverPublicKey, err = crypto_utils.BytesToPublicKey(serverPublicKeyBytes)
	if err != nil {
		panic(err)
	}
}

/*
*Can we lowkey say like "Since CHANGE_PASS must be inside a session
REGISTER
A --> S: {K_AS}K_S,{uid,"REGISTER",pass,r, K_A, {uid,"REGISTER",pass,r}k_A}K_AS

	Server receives and decrypts the request with private key
	Verify the signature signed with client
		Reject if verify fail, Reject =  {uid,"FAIL",r', {uid,"FAIL",r'}k_S}K_A
	Check if user uid already exists
		Reject if exists
	Generate random salt and use it to hash the password and store them with uid

S --> A: {uid,"OK",r, {uid,"OK",r}k_S}K_AS

*
*/
func doRegister(request *Request, response *Response) {
	RegisterSK := crypto_utils.NewSessionKey()
	// A --> S: {K_AS}K_S,{uid,"REGISTER",pass,r, K_A, {uid,"REGISTER",pass,r}k_A}K_AS
	//          ||       ||
	//          ||       ||
	// A --> S: part1   ,part2
	tod := crypto_utils.TodToBytes(crypto_utils.ReadClock())
	message1 := Message{
		Uid:       request.Uid,
		Pass:      request.Pass,
		Status:    "REGISTER",
		PublicKey: crypto_utils.PublicKeyToBytes(&privteKey.PublicKey),
		Time:      tod,
		Values:    nil,
	}
	message1_byte, _ := json.Marshal(message1)
	signture := crypto_utils.Sign(message1_byte, privteKey)
	message2 := Message{
		Uid:       request.Uid,
		Pass:      request.Pass,
		Status:    "REGISTER",
		PublicKey: crypto_utils.PublicKeyToBytes(&privteKey.PublicKey),
		Time:      tod,
		Values:    signture,
	}
	message2_byte, _ := json.Marshal(message2)
	part1 := crypto_utils.EncryptPK(RegisterSK, serverPublicKey)
	part2 := crypto_utils.EncryptSK(message2_byte, RegisterSK)
	connectionMessage := append(part1, part2...)
	//S --> A: {r}KAS
	session_response := sendAndReceive(NetworkData{Payload: connectionMessage, Name: name})
	var serverResponse Response

	errors := json.Unmarshal(session_response.Payload, &serverResponse)
	// fmt.Println("here?", errors != nil, serverResponse)

	if errors != nil {
		var serverResponse Message

		decrypt_session_response, _ := crypto_utils.DecryptSK(session_response.Payload, RegisterSK)
		json.Unmarshal(decrypt_session_response, &serverResponse)
		verify_message := serverResponse
		verify_message.Values = nil
		verify_message_byte, _ := json.Marshal(verify_message)

		tod_plus := serverResponse.Time
		status := serverResponse.Status
		signture := serverResponse.Values
		if crypto_utils.Verify(signture, crypto_utils.Hash(verify_message_byte), serverPublicKey) {
			// tod_plus = serverResponse.Val.([]byte)
			response.Uid = serverResponse.Uid

			if tod_plus != nil && crypto_utils.BytesToTod(tod).Compare(crypto_utils.BytesToTod(tod_plus)) == -1 {
				// fmt.Println("connection finshed")
				if status == "OK" {
					response.Status = OK
				}
			}
		}
	} else {
		// paintext failed
		// fmt.Println("1serverResponse", serverResponse)
		response.Status = serverResponse.Status
	}
}

/*
*

	LOGIN
	A --> S: {K_AS}K_S,{uid,"LOGIN",pass,K_A,r,{uid,"LOGIN",pass, K_A,r}k_A}K_AS
	Server receives and decrypts the session key with private key and use the session to decrypt the login request, obtain password
	Verify the signature signed with client
		Reject if verify fail, Reject =  {uid,"FAIL",r}
	Hash the password and verify if the hash if match to the hash that uid registered
		Reject if verify fail
	S --> A: {uid,"OK",r, {uid,"OK",r}k_S}K_AS

*
*/
func doLogin(request *Request, response *Response) {

	// doOp(request, response)
	// if response.Status != OK {
	// 	break
	// }

	// new session key
	sessionKey = crypto_utils.NewSessionKey()
	// A --> S: {K_AS}Ks,{uid,"LOGIN",pass,K_A,r,{uid,"LOGIN",pass,K_A,r}k_A}K_AS
	//          ||       ||
	//          ||       ||
	// A --> S: part1   ,part2
	tod := crypto_utils.TodToBytes(crypto_utils.ReadClock())
	message1 := Message{Uid: request.Uid, Pass: request.Pass, Status: "LOGIN", PublicKey: crypto_utils.PublicKeyToBytes(&privteKey.PublicKey), Time: tod, Values: nil}
	// var message3 Message
	message1_byte, _ := json.Marshal(message1)
	// json.Unmarshal(message1_byte, &message3)
	signture := crypto_utils.Sign(message1_byte, privteKey)
	message2 := Message{Uid: request.Uid, Pass: request.Pass, Status: "LOGIN", PublicKey: crypto_utils.PublicKeyToBytes(&privteKey.PublicKey), Time: tod, Values: signture}
	message2_byte, _ := json.Marshal(message2)

	// part1 is 256byte
	part1 := crypto_utils.EncryptPK(sessionKey, serverPublicKey)
	part2 := crypto_utils.EncryptSK(message2_byte, sessionKey)
	connectionMessage := append(part1, part2...)
	//S --> A: {r}KAS
	session_response := sendAndReceive(NetworkData{Payload: connectionMessage, Name: name})
	var serverResponse Response

	errors := json.Unmarshal(session_response.Payload, &serverResponse)
	if errors != nil {
		var serverResponse Message

		decrypt_session_response, _ := crypto_utils.DecryptSK(session_response.Payload, sessionKey)

		json.Unmarshal(decrypt_session_response, &serverResponse)
		verify_message := serverResponse
		verify_message.Values = nil
		verify_message_byte, _ := json.Marshal(verify_message)

		tod_plus := serverResponse.Time
		uid := serverResponse.Uid
		status := serverResponse.Status
		signture := serverResponse.Values
		response.Uid = serverResponse.Uid

		if crypto_utils.Verify(signture, crypto_utils.Hash(verify_message_byte), serverPublicKey) {
			// tod_plus = serverResponse.Val.([]byte)

			if tod_plus != nil && crypto_utils.BytesToTod(tod).Compare(crypto_utils.BytesToTod(tod_plus)) == -1 {
				// fmt.Println("connection finshed")
				sessionUID = uid
				if status == "OK" {
					response.Status = OK
				}
			}
		}
	} else {
		// paintext failed
		// fmt.Println("1serverResponse", serverResponse)
		response.Status = serverResponse.Status
		response.Uid = serverResponse.Uid

	}
}

func ProcessOp(request *Request) *Response {
	response := &Response{Status: FAIL}
	if validateRequest(request) {
		switch request.Op {
		case CREATE, DELETE, READ, WRITE, COPY, CHANGE_PASS, MODACL, REVACL:
			request.Uid = sessionUID //no effect if sessionUID not set
			// if sessionUID is set
			// if sessionUID != "" {
			doOp(request, response)
			// }
		case REGISTER:
			if sessionUID == "" {
				doRegister(request, response)
			} else {
				doOp(request, response)
			}
		case LOGIN:

			if sessionUID == "" {
				doLogin(request, response)

			} else {
				doOp(request, response)
			}
		case LOGOUT:
			doOp(request, response)
			// if response.Status == OK { //reset only if successfully logged out
			sessionUID = ""
			// }

		default:
			// struct already default initialized to
			// FAIL status
		}
	}
	return response
}

func validateRequest(r *Request) bool {
	switch r.Op {
	case CREATE, WRITE:
		return r.Key != "" && r.Val != nil
	case DELETE, READ:
		return r.Key != ""
	case COPY:
		return r.Src_key != "" && r.Dst_key != ""
	case REGISTER:
		return r.Uid != "" && r.Pass != ""
	case CHANGE_PASS:
		return r.Old_pass != "" && r.New_pass != ""
	case LOGIN:
		return r.Uid != "" && r.Pass != ""
		// return sessionUID == "" && r.Uid != ""
	case LOGOUT:
		return sessionUID != ""
	case MODACL:
		return sessionUID != ""
	case REVACL:
		return sessionUID != ""
	default:
		return false
	}
}

func doOp(request *Request, response *Response) {
	requestBytes, _ := json.Marshal(request)
	// fmt.Println("-----C:", request.Op, sessionUID, sessionKey)
	if sessionUID != "" {
		requestBytes = crypto_utils.EncryptSK(requestBytes, sessionKey)
		encrypt_response := sendAndReceive(NetworkData{Payload: requestBytes, Name: name}).Payload
		decrypt_response, _ := crypto_utils.DecryptSK(encrypt_response, sessionKey)
		json.Unmarshal(decrypt_response, &response)

	} else {
		json.Unmarshal(sendAndReceive(NetworkData{Payload: requestBytes, Name: name}).Payload, &response)

	}
}

func sendAndReceive(toSend NetworkData) NetworkData {
	Requests <- toSend
	return <-Responses
}
