package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"text/template"
)

type Params struct {
	Args []string
	Arg  string
}

func getParams(fname string) ([]Params, error) {
	f, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)

	all, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	params := make([]Params, 0, len(all))
	for _, args := range all {
		if len(args) == 0 {
			continue
		}
		params = append(params, Params{
			Args: args,
			Arg:  args[0],
		})
	}

	return params, nil
}

func aggregateLines(feeds map[string]io.Reader) <-chan string {
	wg := sync.WaitGroup{}
	c := make(chan string, 1000)

	for name, reader := range feeds {
		wg.Add(1)
		go func(name string, reader io.Reader) {
			buf := bufio.NewScanner(reader)

			for buf.Scan() {
				line := buf.Text()
				c <- fmt.Sprintf("[%s] %s", name, line)
			}
			if err := buf.Err(); err != nil {
				fmt.Println(err)
			}
			wg.Done()
		}(name, reader)
	}

	go func() {
		wg.Wait()
		fmt.Println("done")
		close(c)
	}()

	return c
}

func genCommands(cmd string, params []Params) ([]ShellCmd, error) {
	tmpl, err := template.New("cmd").Parse(cmd)
	if err != nil {
		return nil, err
	}

	cmds := make([]ShellCmd, 0, len(params))
	for _, p := range params {
		buf := &bytes.Buffer{}
		err = tmpl.Execute(buf, p)
		if err != nil {
			return cmds, err
		}
		cmds = append(cmds, ShellCmd{
			cmd:  buf.String(),
			args: p.Args,
		})
	}

	return cmds, nil
}

type Cmd struct {
	*exec.Cmd
	Args   []string
	Stdout io.Reader
	Stderr io.Reader
}

type ShellCmd struct {
	cmd  string
	args []string
}

type Commands struct {
	cmds []*Cmd
	done chan struct{}
	errs []error
}

func (c Commands) getReadersMap() map[string]io.Reader {
	m := make(map[string]io.Reader, len(c.cmds)*2)

	for _, cmd := range c.cmds {
		m[cmd.Args[0]+" (stdout)"] = cmd.Stdout
		m[cmd.Args[0]+" (stderr)"] = cmd.Stderr
	}

	return m
}

func (c Commands) aggregate() {
	m := c.getReadersMap()
	if len(m) == 0 {
		return
	}
	lines := aggregateLines(m)

	go func() {
		for line := range lines {
			fmt.Println(line)
		}
	}()
}

func execute(ctx context.Context, cmds []ShellCmd) (*Commands, error) {
	var err error
	wg := sync.WaitGroup{}
	commands := &Commands{
		done: make(chan struct{}),
	}

	defer func() {
		go func() {
			if len(commands.cmds) != 0 {
				wg.Wait()
			}
			commands.done <- struct{}{}
			close(commands.done)
		}()
	}()
	for _, cmd := range cmds {
		c := &Cmd{
			Cmd:  exec.CommandContext(ctx, "sh", "-c", cmd.cmd),
			Args: cmd.args,
		}
		c.Stdout, err = c.Cmd.StdoutPipe()
		if err != nil {
			return commands, err
		}
		c.Stderr, err = c.Cmd.StderrPipe()
		if err != nil {
			return commands, err
		}
		err := c.Start()
		if err != nil {
			return commands, err
		}
		fmt.Printf("executing %q\n", cmd)
		wg.Add(1)
		go func(c *exec.Cmd) {
			err := c.Wait()
			if err != nil {
				commands.errs = append(commands.errs, err)
			}
			wg.Done()
		}(c.Cmd)
		commands.cmds = append(commands.cmds, c)
	}

	return commands, nil
}

func main() {
	cmd := flag.String("cmd", "echo {{.Arg}}", "command template")
	input := flag.String("input", "/dev/stdin", "input file (csv)")
	flag.Parse()

	params, err := getParams(*input)
	if err != nil {
		panic(err)
	}
	cmds, err := genCommands(*cmd, params)
	if err != nil {
		panic(err)
	}

	commands, err := execute(context.Background(), cmds)
	commands.aggregate()
	<-commands.done
	if err != nil {
		panic(err)
	}
	if len(commands.errs) != 0 {
		fmt.Printf("%+v\n", commands.errs)
	}
}
