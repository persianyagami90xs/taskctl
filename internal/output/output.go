package output

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/taskctl/taskctl/internal/task"
)

const (
	OutputFormatRaw      = "raw"
	OutputFormatPrefixed = "prefixed"
	OutputFormatCockpit  = "cockpit"
)

var Stdout io.Writer = os.Stdout
var Stderr io.Writer = os.Stderr

type DecoratedOutputWriter interface {
	Write(b []byte) (int, error)
	WriteHeader(t *task.Task) error
	WriteFooter(t *task.Task) error
	ForTask(t *task.Task) DecoratedOutputWriter
}

type TaskOutput struct {
	decorator DecoratedOutputWriter
	lock      sync.Mutex
	closeCh   chan bool
}

func NewTaskOutput(format string) (*TaskOutput, error) {
	o := &TaskOutput{
		closeCh: make(chan bool),
	}

	switch format {
	case OutputFormatRaw:
		o.decorator = NewRawOutputWriter(Stdout)
	case OutputFormatPrefixed:
		o.decorator = NewPrefixedOutputWriter(Stdout)
	case OutputFormatCockpit:
		o.decorator = NewCockpitOutputWriter(Stdout, o.closeCh)
	default:
		return nil, fmt.Errorf("unknown decorator \"%s\" requested", format)
	}

	return o, nil
}

func (o *TaskOutput) Stream(t *task.Task, cmdStdout, cmdStderr io.ReadCloser, out chan []byte) {
	o.lock.Lock()

	var wg sync.WaitGroup
	wg.Add(2)

	decorator := o.decorator.ForTask(t)

	buf := bytes.Buffer{}
	go func(dst io.Writer) {
		defer wg.Done()
		err := o.pipe(dst, cmdStdout)
		if err != nil {
			logrus.Debug(err)
		}
	}(io.MultiWriter(&buf, decorator, &t.Log.Stdout))

	go func(dst io.Writer) {
		defer wg.Done()
		err := o.pipe(dst, cmdStderr)
		if err != nil {
			logrus.Debug(err)
		}
	}(io.MultiWriter(&buf, decorator, &t.Log.Stderr))

	o.lock.Unlock()

	wg.Wait()
	out <- buf.Bytes()
	close(out)
}

func (o *TaskOutput) pipe(dst io.Writer, src io.ReadCloser) error {
	var buf = make([]byte, 1)
	var err error
	for {
		_, err = src.Read(buf)
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}

		_, err = dst.Write(buf)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *TaskOutput) Start(t *task.Task) error {
	return o.decorator.WriteHeader(t)
}

func (o *TaskOutput) Finish(t *task.Task) error {
	return o.decorator.WriteFooter(t)
}

func (o *TaskOutput) Close() {
	close(o.closeCh)
}

func SetStdout(w io.Writer) {
	Stdout = w
}
func SetStderr(w io.Writer) {
	Stderr = w
}