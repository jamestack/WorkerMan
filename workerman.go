package workerman

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"runtime"
	"time"
)

//Worker Struct
type Worker struct {
	OnConnect func(*Client)
	OnMessage func(client *Client, message string)
	OnClose   func(*Client)
	clients   []*Client
	address   string
	index     uint64
}

func (worker *Worker) SendToAll(msg string) {
	for _, client := range worker.clients {
		client.SendMessage(msg)
	}
}

//func (worker *Worker) CloseAll() {
//	for i, client := range worker.clients {
//		client.Close()
//		worker.clients[i] = nil
//	}
//}

func (worker *Worker) SendToUid(uid string, msg string) (err error) {
	for _, client := range worker.clients {
		if client.Uid == uid {
			err = client.SendMessage(msg)
			return err
		}
	}
	return errors.New("Unknow Uid")
}

func (worker *Worker) SendToClient(cid string, msg string) (err error) {
	for _, client := range worker.clients {
		if client.Clientid == cid {
			err = client.SendMessage(msg)
			return
		}
	}
	return errors.New("Unknow Clientid")
}

//func (worker *Worker) SendToGroup(gid string, msg string) {
//	for _, client := range worker.clients {
//		if isExist, ok := client.group[gid]; ok && isExist {
//			client.SendMessage(msg)
//		}
//	}
//}

//func (worker *Worker) CloseAllByGroup(gid string, msg string) {
//	for i, client := range worker.clients {
//		if isExist, ok := client.group[gid]; ok && isExist {
//			client.Close()
//			worker.clients[i] = nil
//		}
//	}
//}

//func (worker *Worker) JoinGroup(gid string, clientid string) {
//	for _, client := range worker.clients {
//		client.group[gid] = true
//	}
//}

//func (worker *Worker) LeaveGroup(gid string, clientid string) {
//	for _, client := range worker.clients {
//		if isExist, ok := client.group[gid]; ok && isExist {
//			client.group[gid] = true
//		}
//	}
//}

//Client Struct
type Client struct {
	Uid        string
	Clientid   string
	con        *net.TCPConn
	group      map[string]bool
	remoteAddr string
	worker     *Worker
}

func (client Client) SendMessage(msg string) (err error) {
	err = sendMsg(client.con, []byte(msg))
	if err != nil {
		return err
	}
	return nil
}

func (client *Client) Close() {
	client.con.Close()
}

func (client *Client) BindUserid(uid string) {
	client.Uid = uid
}

//func (client *Client) JoinGroup(gid string) {
//	client.group[gid] = true
//}

//func (client *Client) LeaveGroup(gid string) {
//	if _, ok := client.group[gid]; ok {
//		client.group[gid] = false
//		return
//	}
//}

func (client *Client) RemoteAddr() (ipaddr string) {
	return client.remoteAddr
}

//Websocket decode
func (worker *Worker) onAccept(con *net.TCPConn) {
	var client *Client
	var bindata []byte
	isHanded := false
	var response string
	headder := make(map[string]string)
	for {
		//设置缓冲区为512
		bit := make([]byte, 512)
		num, err := con.Read(bit)
		if err != nil {
			fmt.Println("read err.", err.Error())
			break
		}
		if num > 0 {
			for i := 0; i < num; i++ {
				bindata = append(bindata, bit[i])
			}
		}
		if num < 512 {
			if isHanded {
				//解析数据帧
				fmt.Println("New Data:")
				err, data := decode(bindata)
				//生成回调
				if err != nil {
					switch err.Error() {
					case "text":
						worker.OnMessage(client, string(data))
					case "binary":
						fmt.Println(data)
					case "ping":
						fmt.Println("onPing")
					case "pong":
						fmt.Println("onPong")
					case "close":
						fmt.Println("onClose")
					case "other opcode":
						fmt.Println("on Other Opcode")
					default:
						fmt.Println("err:", err.Error())
					}
				}
			} else {
				//Websocket握手
				response = string(bindata)
				if headpath := regexp.MustCompile(`(?:^)([^\r\n]+)(?:\r\n)`).FindAllStringSubmatch(response, -1); len(headpath) > 0 {
					headder["HeadPath"] = headpath[0][1]
				}
				if path := regexp.MustCompile(`(?:GET\s)(.+)(?:\sHTTP\/\d)`).FindAllStringSubmatch(response, -1); len(path) > 0 {
					headder["path"] = path[0][1]
				}
				headtitle := []string{"Host", "Connection", "Pragma", "Cache-Control", "Upgrade", "Origin", "Sec-WebSocket-Version", "DNT", "User-Agent", "Accept-Language", "Sec-WebSocket-Key", "Sec-WebSocket-Extensions"}
				for _, title := range headtitle {
					if reg := regexp.MustCompile(`(?:`+title+`:\s)(.+)(?:\r\n)`).FindAllStringSubmatch(response, -1); len(reg) > 0 {
						headder[title] = reg[0][1]
					}
				}
				if _, ok := headder["Sec-WebSocket-Key"]; ok {
					sha := sha1.New()
					sha.Write([]byte(headder["Sec-WebSocket-Key"]))
					sha.Write([]byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
					b64 := base64.StdEncoding.EncodeToString(sha.Sum(nil))
					retstr := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + b64 + "\r\n\r\n"
					con.Write([]byte(retstr))
					isHanded = true
					client = worker.addClient(con)
					worker.OnConnect(client)
				}
			}
			bindata = []byte{}
		}
	}
}

func (worker *Worker) addClient(con *net.TCPConn) (client *Client) {
	client = &Client{Uid: string(0), Clientid: fmt.Sprintf("%d", worker.index+1), con: con, group: map[string]bool{}, remoteAddr: con.RemoteAddr().String(), worker: worker}
	worker.index++
	worker.clients = append(worker.clients, client)
	return
}

func sendMsg(con *net.TCPConn, data []byte) (err error) {
	lenCode := len(data)
	var payloadData []byte
	rand.Seed(time.Now().Unix())
	bytebuff := bytes.NewBuffer([]byte{})
	binary.Write(bytebuff, binary.BigEndian, rand.Int31())
	bytebuff.Reset()
	binary.Write(bytebuff, binary.BigEndian, uint8(129))
	head := bytebuff.Bytes()[0]

	var payloadLen byte
	var extendPayload []byte
	switch {
	case lenCode < 126:
		bytebuff.Reset()
		binary.Write(bytebuff, binary.BigEndian, uint8(lenCode))
		payloadLen = bytebuff.Bytes()[0]
	case lenCode < 65536:
		bytebuff.Reset()
		binary.Write(bytebuff, binary.BigEndian, uint8(126))
		payloadLen = bytebuff.Bytes()[0]

		bytebuff.Reset()
		binary.Write(bytebuff, binary.BigEndian, uint16(lenCode))
		extendPayload = bytebuff.Bytes()

	case lenCode < 131072:
		bytebuff.Reset()
		binary.Write(bytebuff, binary.BigEndian, uint8(127))
		payloadLen = bytebuff.Bytes()[0]

		bytebuff.Reset()
		binary.Write(bytebuff, binary.BigEndian, uint64(lenCode))
		extendPayload = bytebuff.Bytes()
	default:
		return errors.New("data too lang.")
	}
	payloadData = append(payloadData, head, payloadLen)
	if len(extendPayload) > 0 {
		for i := 0; i < len(extendPayload); i++ {
			payloadData = append(payloadData, extendPayload[i])
		}
	}
	for i := 0; i < lenCode; i++ {
		payloadData = append(payloadData, data[i])
	}
	fmt.Println("send:", payloadData)
	con.Write(payloadData)
	return nil
}

func decode(code []byte) (err error, payloadData []byte) {
	//初始化变量
	lenCode := len(code)
	if lenCode < 2 {
		return errors.New("bindata length too short(" + string(lenCode) + ")."), []byte{}
	}
	fine := code[0] >> 7
	if fine == 0x0 {
		//拒绝不完整的帧
		return errors.New("not fine"), []byte{}
	}
	opcode := code[0] << 4 >> 4
	var hasMask bool = false
	var maskKey []byte
	var payloadLen uint64
	payloadLen = uint64(code[1] << 1 >> 1)
	//解析数据长度
	switch payloadLen {
	case 126:
		//解析payloadLen为uint16
		if lenCode < 4 {
			return errors.New("bindata length too short(" + string(lenCode) + ")."), []byte{}
		}
		bytebuff := bytes.NewReader(code[2:4])
		var tmp uint16
		err := binary.Read(bytebuff, binary.BigEndian, &tmp)
		payloadLen = uint64(tmp)
		if err != nil {
			return errors.New("uint16 payloadLen err.1"), []byte{}
		}
		if code[1]>>7 == 0x1 {
			if uint64(lenCode) < 8+payloadLen {
				return errors.New("uint16 payloadLen too short.2"), []byte{}
			}
			hasMask = true
			maskKey = code[4:8]
			payloadData = code[8 : 8+payloadLen]
		} else {
			if uint64(lenCode) < 4+payloadLen {
				return errors.New("uint16 payloadLen too short.3"), []byte{}
			}
			payloadData = code[4 : 4+payloadLen]
		}
	case 127:
		//解析payloadLen为uint64
		if lenCode < 10 {
			return errors.New("uint64 payloadLen too short.1"), []byte{}
		}
		bytebuff := bytes.NewReader(code[2:10])
		err := binary.Read(bytebuff, binary.BigEndian, &payloadLen)
		if err != nil {
			return errors.New("uint64 payloadLen err.2"), []byte{}
		}
		if code[1]>>7 == 0x1 {
			if uint64(lenCode) < 14+payloadLen {
				return errors.New("uint64 payloadLen too short.3"), []byte{}
			}
			hasMask = true
			maskKey = code[10:14]
			payloadData = code[14 : 14+payloadLen]
		} else {
			if uint64(lenCode) < 10+payloadLen {
				return errors.New("uint64 payloadLen too short.4"), []byte{}
			}
			payloadData = code[10 : 10+payloadLen]
		}
	default:
		//解析payloadLen为7 bit
		if uint64(lenCode) < 6+payloadLen {
			return errors.New("uint64 payloadLen too short.5"), []byte{}
		}
		payloadData = code[6 : 6+payloadLen]
		if code[1]>>7 == 0x1 {
			hasMask = true
			maskKey = code[2:6]
		}
	}
	switch fine {
	case 0x0:
		//拒绝非完整的帧
		return errors.New("not fine"), []byte{}
	case 0x1:
		switch opcode {
		case 0x0:
			//附加数据帧，由于数据不完整，手动拼接容易损坏数据，所以拒绝接收。
			return errors.New("not fine"), []byte{}
		case 0x1:
			//为文本数据
			if hasMask {
				//掩码解密
				for i, _ := range payloadData {
					payloadData[i] = payloadData[i] ^ maskKey[i%4]
				}
			}
			return errors.New("text"), payloadData
		case 0x2:
			//为二进制数据
			if hasMask {
				//掩码解密
				for i, _ := range payloadData {
					payloadData[i] = payloadData[i] ^ maskKey[i%4]
				}
			}
			return errors.New("binary"), payloadData
		case 0x8:
			//连接关闭
			return errors.New("close"), []byte{}
		case 0x9:
			//代表ping
			return errors.New("ping"), []byte{}
		case 0xA:
			//代表pong
			return errors.New("pong"), []byte{}
		default:
			//Other Opcode
		}
	}
	return errors.New("other opcode"), []byte{}
}

func NewWorker(addr string) (worker *Worker) {
	return &Worker{OnConnect: nil, OnMessage: nil, OnClose: nil, clients: nil, address: addr, index: uint64(0)}
}

//websocket run
func (worker *Worker) Run() (err error) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	localAddr, err := net.ResolveTCPAddr("tcp", worker.address)
	tcpListener, err := net.ListenTCP("tcp", localAddr)
	if err != nil {
		return errors.New("Tcp Listen fail.")
	}
	for {
		tcpCon, err := tcpListener.AcceptTCP()
		if err != nil {
			fmt.Println("tcpCon Fail.")
			return errors.New("Tcp Connection fail.")
		}
		fmt.Println("New Connect:", tcpCon.RemoteAddr())
		go worker.onAccept(tcpCon)
	}
}
