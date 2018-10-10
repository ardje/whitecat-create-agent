/*
 * Whitecat Blocky Environment, Whitecat Agent Websocket Server
 *
 * Copyright (C) 2015 - 2016
 * IBEROXARXA SERVICIOS INTEGRALES, S.L.
 *
 * Author: Jaume Olivé (jolive@iberoxarxa.com / jolive@whitecatboard.org)
 *
 * All rights reserved.
 *
 * Permission to use, copy, modify, and distribute this software
 * and its documentation for any purpose and without fee is hereby
 * granted, provided that the above copyright notice appear in all
 * copies and that both that the copyright notice and this
 * permission notice and warranty disclaimer appear in supporting
 * documentation, and that the name of the author not be used in
 * advertising or publicity pertaining to distribution of the
 * software without specific, written prior permission.
 *
 * The author disclaim all warranties with regard to this
 * software, including all implied warranties of merchantability
 * and fitness.  In no event shall the author be liable for any
 * special, indirect or consequential damages or any damages
 * whatsoever resulting from loss of use, data or profits, whether
 * in an action of contract, negligence or other tortious action,
 * arising out of or in connection with the use or performance of
 * this software.
 */

package main

/*

This is the implementation for the Whitecat Agent websocket server. This server has two main
functions:

* Listen commands sended by the IDE, process this commands, and send back a response to the IDE
* Notify IDE about important things that it must be know

Notifications:

{"notify": "boardAttached", "info": {"modules":[], "maps": []}}
{"notify": "boardDetached", "info": {}}
{"notify": "boardPowerOnReset", "info": {}}
{"notify": "boardSoftwareReset", "info": {}}
{"notify": "boardDeepSleepReset", "info": {}}
{"notify": "boardRuntimeError", "info": {"where": "xx", "line": "xx", "exception": "xx", "message": "xx"}}
{"notify": "boardConsoleOut", "info": {"content": "xxx"}}
{"notify": "boardUptate", "info": {}}
{"notify": "boardUpgraded", "info": {}}
{"notify": "boardTimeout", "info": {}}
{"notify": "invalidFirmware", "info": {}}

Available commands:

{"command": "attachIde", "arguments": "{}"}
{"command": "detachIde", "arguments": "{}"}

{"command": "boardUpgrade", "arguments": "{}"}
{"command": "boardInfo", "arguments": "{}"}
{"command": "boardReset, "arguments": "{}"}
{"command": "boardStop, "arguments": "{}"}
{"command": "boardGetDirContent", "arguments": {"path": "xxxx"}}
{"command": "boardReadFile", "arguments": {"path": "xxxx"}}
{"command": "boardRunProgram", "arguments": {"path": "xxxx", "code": "xxxx"}}
{"command": "boardRunCommand", "arguments": {"code": "xxxx"}}
{"command": "boardInstall", "arguments": {"firmware": "xxxx"}}

*/

import (
	"encoding/base64"
	"encoding/json"
	"golang.org/x/net/websocket"
	"log"
	"net/http"
	"os"
	"time"
)

var IdeDetach chan bool

var ConsoleUp chan byte

var ControlWs *websocket.Conn = nil
var UpWs *websocket.Conn = nil

type deviceDef struct {
	VendorId  string
	ProductId string
	Vendor    string
}

var devices []deviceDef

type Command struct {
	Command string
}

type CommandFileSystem struct {
	Command   string
	Arguments struct {
		Path    string
		Content string
	}
}

type CommandRunProgram struct {
	Command   string
	Arguments struct {
		Path string
		Code string
	}
}

type CommandRunCommand struct {
	Command   string
	Arguments struct {
		Path string
		Code string
	}
}

type CommandInstallCommand struct {
	Command   string
	Arguments struct {
		Firmware string
	}
}

type AttachIdeCommand struct {
	Command   string
	Arguments struct {
		Devices []deviceDef
	}
}

func notify(notification string, data string) {
	var err error
	var msg string
	var info string = "{}"

	// Build info for each notification type
	switch notification {
	case "boardAttached":
		newBuild := "false"
		if connectedBoard.newBuild {
			newBuild = "true"
		}

		info = "{\"info\": " + connectedBoard.info + ", \"newBuild\": " + newBuild + "}"

	case "blockStart":
		info = "{" + data + "}"

	case "blockEnd":
		info = "{" + data + "}"

	case "blockError":
		info = "{" + data + "}"

	case "boardRuntimeError":
		info = "{" + data + "}"

	case "boardRuntimeWarning":
		info = "{" + data + "}"

	case "boardGetDirContent":
		info = data

	case "boardReadFile":
		info = "{\"content\": \"" + data + "\"}"

	case "boardConsoleOut":
		info = "{\"content\": \"" + data + "\"}"

	case "boardRunCommand":
		info = "{\"response\": \"" + data + "\"}"

	case "boardUpdate":
		info = "{\"what\": \"" + base64.StdEncoding.EncodeToString([]byte(data)) + "\"}"

	case "attachIde":
		info = "{\"agent-version\": \"" + Version + "\"}"
	}

	// Build message
	msg = "{\"notify\": \"" + notification + "\", \"info\": " + info + "}"

	// Send message
	if ControlWs != nil {
		if err = websocket.Message.Send(ControlWs, msg); err != nil {
		}
		log.Println("notify: ", msg)
	} else {
		log.Println("can't notify: ", msg)
	}
}

func control(ws *websocket.Conn) {
	var msg string
	var err error
	var command Command

	ControlWs = ws

	log.Println("start control ...")

	defer func() {
		ws.Close()
		log.Println("stop control ...")
	}()

	for {
		if Upgrading {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		// Get a new message
		if err = websocket.Message.Receive(ws, &msg); err != nil {
			return
		}

		if Upgrading {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		log.Println("received message: ", msg)

		// Parse command
		json.Unmarshal([]byte(msg), &command)

		switch command.Command {
		case "attachIde":
			if connectedBoard == nil {
				var attachIdeCommand AttachIdeCommand

				json.Unmarshal([]byte(msg), &attachIdeCommand)

				connectedBoard.detach()
				notify("attachIde", "")
				devices = attachIdeCommand.Arguments.Devices
				go monitor()
			} else {
				connectedBoard.reset(false)
				notify("attachIde", "")
				notify("boardAttached", "")
			}

		case "detachIde":
			IdeDetach <- true
			IdeDetach <- true
			connectedBoard.detach()

			return

		case "boardReset":
			if connectedBoard != nil {
				notify("boardUpdate", "Reseting board")
				connectedBoard.reset(false)
				notify("boardReset", "")
				notify("boardAttached", "")
			}

		case "boardStop":
			if connectedBoard != nil {
				notify("boardUpdate", "Stopping program")
				connectedBoard.reset(false)
				notify("boardReset", "")
				notify("boardAttached", "")
			}

		case "boardGetDirContent":
			if connectedBoard != nil {
				var fsCommand CommandFileSystem

				json.Unmarshal([]byte(msg), &fsCommand)

				dirContent := connectedBoard.getDirContent(fsCommand.Arguments.Path)
				if dirContent == "" {
					// getDirContent has failed, probably because the main thread is executing
					// a blocking program.
					//
					// stop program, and retry

					notify("boardUpdate", "Stopping program")
					connectedBoard.reset(false)
					notify("boardReset", "")
					notify("boardAttached", "")

					dirContent = connectedBoard.getDirContent(fsCommand.Arguments.Path)
					if dirContent == "" {
						// Ooops, something is wrong
						notify("boardGetDirContent", "[]")
						notify("boardTimeout", "")
					} else {
						notify("boardGetDirContent", dirContent)
					}
				} else {
					notify("boardGetDirContent", dirContent)
				}
			}

		case "boardReadFile":
			if connectedBoard != nil {
				var fsCommand CommandFileSystem

				json.Unmarshal([]byte(msg), &fsCommand)

				fileContent := connectedBoard.readFile(fsCommand.Arguments.Path)
				if fileContent == nil {
					// readFile has failed, probably because the main thread is executing
					// a blocking program.
					//
					// stop program, and retry

					notify("boardUpdate", "Stopping program")
					connectedBoard.reset(false)
					notify("boardReset", "")
					notify("boardAttached", "")

					fileContent = connectedBoard.readFile(fsCommand.Arguments.Path)
					if fileContent == nil {
						// Ooops, something is wrong
						notify("boardReadFile", base64.StdEncoding.EncodeToString(fileContent))
						notify("boardTimeout", "")
					} else {
						notify("boardReadFile", base64.StdEncoding.EncodeToString(fileContent))
					}
				} else {
					notify("boardReadFile", base64.StdEncoding.EncodeToString(fileContent))
				}
			}

		case "boardWriteFile":
			if connectedBoard != nil {
				var fsCommand CommandFileSystem

				json.Unmarshal([]byte(msg), &fsCommand)

				content, err := base64.StdEncoding.DecodeString(fsCommand.Arguments.Content)
				if err == nil {
					ret := connectedBoard.writeFile(fsCommand.Arguments.Path, content)
					if ret == "" {
						// writeFile has failed, probably because the main thread is executing
						// a blocking program.
						//
						// stop program, and retry

						notify("boardUpdate", "Stopping program")
						connectedBoard.reset(false)
						notify("boardReset", "")
						notify("boardAttached", "")

						ret = connectedBoard.writeFile(fsCommand.Arguments.Path, content)
						if ret == "" {
							// Ooops, something is wrong
							notify("boardWriteFile", "")
							notify("boardTimeout", "")
						} else {
							notify("boardWriteFile", "")
						}
					} else {
						notify("boardWriteFile", "")
					}
				}
			}

		case "boardRemoveFile":
			if connectedBoard != nil {
				var fsCommand CommandFileSystem

				json.Unmarshal([]byte(msg), &fsCommand)

				path, err := base64.StdEncoding.DecodeString(fsCommand.Arguments.Path)
				if err == nil {
					connectedBoard.removeFile(string(path))
					notify("boardRemoveFile", "")
				}
			}

		case "boardRunProgram":
			if connectedBoard != nil {
				var runCommand CommandRunProgram

				json.Unmarshal([]byte(msg), &runCommand)

				code, err := base64.StdEncoding.DecodeString(runCommand.Arguments.Code)
				if err == nil {
					connectedBoard.runProgram(runCommand.Arguments.Path, []byte(code))
					notify("boardRunProgram", "")
				}
			}

		case "boardRunCommand":
			if connectedBoard != nil {
				var runCommand CommandRunCommand

				json.Unmarshal([]byte(msg), &runCommand)

				code, err := base64.StdEncoding.DecodeString(runCommand.Arguments.Code)
				if err == nil {
					connectedBoard.runCode(code)
					response := connectedBoard.runCommand([]byte("_code()"))
					notify("boardRunCommand", base64.StdEncoding.EncodeToString([]byte(response)))
				}
			}

		case "boardUpgrade":
			if connectedBoard != nil {
				connectedBoard.upgrade(false, "")
				notify("boardUpgraded", "")
			}

		case "boardInstall":
			if connectedBoard != nil && !connectedBoard.validFirmware {
				var installCommand CommandInstallCommand

				json.Unmarshal([]byte(msg), &installCommand)

				connectedBoard.upgrade(true, installCommand.Arguments.Firmware)
				notify("boardUpgraded", "")
			}
		}
	}
}

func consoleUp(ws *websocket.Conn) {
	var err error

	UpWs = ws

	log.Println("consoleUp start ...")

	defer ws.Close()
	defer log.Println("consoleUp stop ...")

	for {
		select {
		case <-IdeDetach:
			return
		default:
			if len(ConsoleUp) > 0 {
				if Upgrading {
					<-ConsoleUp
					time.Sleep(time.Millisecond * 100)
					continue
				}
				if err = websocket.Message.Send(ws, string(<-ConsoleUp)); err != nil {
					return
				}
			} else {
				time.Sleep(time.Millisecond)
			}
		}
	}
}

func consoleDown(ws *websocket.Conn) {
	var err error
	var msg string

	log.Println("consoleDown start ...")

	defer ws.Close()
	defer log.Println("consoleDown stop ...")

	for {
		select {
		case <-IdeDetach:
			return
		default:
			// Get a new message
			if err = websocket.Message.Receive(ws, &msg); err != nil {
				return
			}

			if Upgrading {
				time.Sleep(time.Millisecond * 100)
				continue
			}

			if connectedBoard != nil {
				connectedBoard.port.Write([]byte(msg))
			}
		}
	}
}

func webSocketStart(exitChan chan int) {
	//generateCertificates()

	ConsoleUp = make(chan byte, 10*1024)
	IdeDetach = make(chan bool)

	http.Handle("/", websocket.Handler(control))
	http.Handle("/control", websocket.Handler(control))
	http.Handle("/up", websocket.Handler(consoleUp))
	http.Handle("/down", websocket.Handler(consoleDown))

	go func() {
		log.Println("AppFolder: ", AppFolder)
		log.Println("AppFileName: ", AppFileName)
		log.Println("AppDataFolder: ", AppDataFolder)
		log.Println("AppDataTmpFolder: ", AppDataTmpFolder)

		log.Println("Starting non secure websocket server ...")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal(err)

			os.Exit(1)
		}
	}()
}
