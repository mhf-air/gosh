package gosh

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

var Env = map[string]string{}

func init() {
	for _, item := range os.Environ() {
		lst := strings.Split(item, "=")
		Env[lst[0]] = lst[1]
	}
}

func ck(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func Pipe(s string) string {
	commandList := scan(s)

	output := new(bytes.Buffer)
	length := len(commandList)
	switch length {
	case 0:
		panic("there is nothing in pipe")
	case 1:
		cmd := exec.Command(commandList[0].Cmd[0], commandList[0].Cmd[1:]...)
		if commandList[0].In == nil {
			cmd.Stdin = os.Stdin
		} else {
			cmd.Stdin = commandList[0].In
		}

		if commandList[0].Out == nil {
			cmd.Stdout = output
		} else {
			cmd.Stdout = commandList[0].Out
		}

		cmd.Stderr = commandList[0].Err
		err := cmd.Run()
		ck(err)
	default:
		cmdList := make([]*exec.Cmd, length)
		bufList := make([]*bytes.Buffer, length-1)
		for i := 0; i < length-1; i++ {
			bufList[i] = new(bytes.Buffer)
		}

		for i, order := range commandList {
			cmd := exec.Command(order.Cmd[0], order.Cmd[1:]...)
			switch i {
			case 0: // first command
				if order.In == nil {
					cmd.Stdin = os.Stdin
				} else {
					cmd.Stdin = order.In
				}
				cmd.Stdout = bufList[i]
				cmd.Stderr = order.Err
			case length - 1: // last command
				cmd.Stdin = bufList[i-1]
				if order.Out == nil {
					cmd.Stdout = output
				} else {
					cmd.Stdout = order.Out
				}
				cmd.Stderr = order.Err
			default:
				cmd.Stdin = bufList[i-1]
				cmd.Stdout = bufList[i]
				cmd.Stderr = order.Err
			}
			cmdList[i] = cmd
		}

		for _, c := range cmdList {
			err := c.Run()
			ck(err)
		}
	}
	return output.String()
}

type SingleCmd struct {
	In  io.Reader // stdin for the first command
	Out io.Writer // stdout for the last command
	Err io.Writer // stderr for all commands
	Cmd []string
}

func scan(s string) []SingleCmd {
	var (
		result    = []SingleCmd{}
		singleCmd = SingleCmd{}
		err       error
	)

	scanner := bufio.NewScanner(strings.NewReader(s))
	scanner.Split(scanBashWords)
	for scanner.Scan() {
		token := scanner.Text()
		switch token {
		case "|":
			result = append(result, singleCmd)
			singleCmd = SingleCmd{}
		case "<":
			if scanner.Scan() {
				fileName := expandPath(scanner.Text())
				singleCmd.In, err = os.OpenFile(fileName, os.O_RDONLY, 0644)
				ck(err)
			}
		case ">":
			if scanner.Scan() {
				fileName := expandPath(scanner.Text())
				singleCmd.Out, err = os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
				ck(err)
			}
		case "^":
			if scanner.Scan() {
				fileName := expandPath(scanner.Text())
				singleCmd.Err, err = os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
				ck(err)
			}
		case ">>":
			if scanner.Scan() {
				fileName := expandPath(scanner.Text())
				singleCmd.Out, err = os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
				ck(err)
			}
		default:
			singleCmd.Cmd = append(singleCmd.Cmd, expandPath(token))
		}
	}
	if len(singleCmd.Cmd) != 0 {
		result = append(result, singleCmd)
	}
	err = scanner.Err()
	ck(err)
	return result
}

func expandPath(s string) string {
	var result string

	switch len(s) {
	case 0:
		result = s
		panic("empty string in expandPath")
	case 1:
		if s == "~" {
			result = Env["HOME"]
		} else {
			result = s
		}
	default:
		switch s[0] {
		case '\'', '"':
			result = s
		default:
			// expand tilda
			if s[1] == '/' {
				result = Env["HOME"] + s[1:]
			} else {
				result = s
			}
		}
	}

	return result
}

func scanBashWords(data []byte, atEOF bool) (advance int, token []byte, err error) {
	var (
		in        = map[string]bool{} // state map
		wordStart int
	)

	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	// fmt.Println("data:" + string(data))

	_ = "breakpoint"
	for i := 0; i < len(data); i++ {
		if in["escape"] {
			in["escape"] = false
			i++
		} else {
			b := data[i]
			if in["single-quote"] {
				switch b {
				case '\'':
					in["single-quote"] = false
					return i + 1, data[wordStart : i+1], nil
				case '\\':
					in["escape"] = true
				}
			} else if in["double-quote"] {
				switch b {
				case '"':
					in["double-quote"] = false
					return i + 1, data[wordStart : i+1], nil
				case '\\':
					in["escape"] = true
				}
			} else {

				// in non-string context
				switch b {
				case '\'':
					if in["word"] {
						return 0, nil, errors.New("string literal in word")
					}
					wordStart = i
					in["single-quote"] = true
				case '"':
					if in["word"] {
						return 0, nil, errors.New("string literal in word")
					}
					wordStart = i
					in["double-quote"] = true
				case ' ':
					if in["word"] {
						return i + 1, data[wordStart:i], nil
					}
				case '|', '<', '^':
					if in["word"] {
						return i, data[wordStart:i], nil
					} else {
						return 1, data[i : i+1], nil
					}
				case '>':
					if data[i+1] == '>' {
						if in["word"] {
							return i + 1, data[wordStart:i], nil
						} else {
							return 2, data[i : i+2], nil
						}
					} else {
						if in["word"] {
							return i + 1, data[wordStart:i], nil
						} else {
							return 1, data[i : i+1], nil
						}
					}
				default:
					if !in["word"] {
						wordStart = i
						in["word"] = true
					}
					// fmt.Println("in default:", string(b), in)
				}

			}
		}
	}

	if atEOF {
		d := bytes.Trim(data, " \r")
		if len(d) > 0 {
			return len(data), d, nil
		}
	}

	return 0, nil, nil
}
