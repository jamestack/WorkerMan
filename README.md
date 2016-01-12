# WorkerMan WebSocket Server Package For Golang
##背景
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;前段时间想用Golang写一个WebSocket的Server，却发现Golang标准包中没有提供WebSocket支持，
呵呵自己动手丰衣足食。便在网上查了下WebSocket协议的资料，自己实现了WebSocket的握手和数据帧解析。
##为什么叫WorkerMan
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;<a href="http://www.workerman.net">WorkerMan</a>是一款纯PHP开发的开源的高性能的PHP socket服务器框架，
因为我工作主要是PHP服务器端开发，所以对WorkerMan相当有感情。我将这个Websocket Package起名叫WorkerMan是向WorkerMan开源团队表示致敬(其实我只是不知道叫啥好..2333)，当然现阶段的WorkerMan
还只是完成了基本的雏形，<strong>没有过多研究语法和效率问题、代码写的也不讲究</strong>，玩玩尚可千万不要用于项目开发中哦！
##进展
beta v1.0 基本功能
##未来打算
我会抽出空余时间不断完善代码的，打算下个版本给WorkerMan加上Client Group功能，下下个版本加上自定义协议功能...想想就好累~
废话不多说，直接看Demo吧。
##Demo
```go
package main

import (
	"workerman"
)

func main() {
	worker := workerman.NewWorker("0.0.0.0:8080")
	worker.OnMessage = func(client *workerman.Client, msg string) {
		switch msg {
		case "quit":
			client.SendMessage("quit success.")
			worker.SendToAll(client.Clientid + " quit this room.")
			client.Close()
		default:
			worker.SendToAll("Client" + client.Clientid + ":" + msg)
		}
	}
	worker.OnConnect = func(client *workerman.Client) {
		client.BindUserid(client.Clientid)
		client.SendMessage("欢迎你：" + client.RemoteAddr())
		worker.SendToAll("欢迎Client:" + client.Clientid + "加入房间")
	}
	worker.Run()
}

```

##WebSocket协议握手实现
```go
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
```
##WebSocket数据帧解析实现

```go
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
```
