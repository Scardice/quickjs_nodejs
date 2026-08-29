package eventloop

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	quickjs "github.com/buke/quickjs-go"
)

// Context is the canonical owner-bound adapter for one EventLoop generation.
// Raw returns a QuickJS context that must only be used on the loop owner.
type Context struct {
	loop       *EventLoop
	raw        *quickjs.Context
	generation uint64
}

// Generation returns the current monotonically increasing context generation.
func (l *EventLoop) Generation() uint64 {
	if l == nil {
		return 0
	}
	return l.generation.Load()
}

// CheckGeneration verifies that generation still refers to the active context.
func (l *EventLoop) CheckGeneration(generation uint64) error {
	if l == nil || l.closed.Load() {
		return ErrClosed
	}
	if l.Generation() != generation {
		return ErrStaleGeneration
	}
	return nil
}

// Raw returns the underlying owner-bound QuickJS context.
func (c *Context) Raw() *quickjs.Context {
	if c == nil {
		return nil
	}
	return c.raw
}

// Eval evaluates JavaScript on the owner goroutine. Call it from a ContextTask,
// DoContext, or RunContext callback.
func (c *Context) Eval(code string, opts ...quickjs.EvalOption) *quickjs.Value {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.Eval(code, opts...)
}

// LoadModule evaluates an ESM source with the supplied module name.
func (c *Context) LoadModule(code, moduleName string, opts ...quickjs.EvalOption) *quickjs.Value {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.LoadModule(code, moduleName, opts...)
}

// Globals returns the global object for this context.
func (c *Context) Globals() *quickjs.Value {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.Globals()
}

// Exception returns the most recent QuickJS exception.
func (c *Context) Exception() error {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.Exception()
}

// Throw throws an existing exception value in this context.
func (c *Context) Throw(value *quickjs.Value) *quickjs.Value {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.Throw(value)
}

// ThrowError creates and throws a generic error in this context.
func (c *Context) ThrowError(err error) *quickjs.Value {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.ThrowError(err)
}

// ThrowTypeError creates and throws a TypeError in this context.
func (c *Context) ThrowTypeError(format string, args ...any) *quickjs.Value {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.ThrowTypeError(format, args...)
}

// ThrowRangeError creates and throws a RangeError in this context.
func (c *Context) ThrowRangeError(format string, args ...any) *quickjs.Value {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.ThrowRangeError(format, args...)
}

// CheckGeneration verifies that generation still refers to the active context.
func (c *Context) CheckGeneration(generation uint64) error {
	if c == nil || c.loop == nil {
		return ErrClosed
	}
	return c.loop.CheckGeneration(generation)
}

// Generation returns the generation that owns the current raw context.
func (c *Context) Generation() uint64 {
	if c == nil || c.loop == nil {
		return 0
	}
	return c.loop.Generation()
}

// EventLoop returns the adapter for the current context generation.
func (l *EventLoop) Context() *Context {
	if l == nil {
		return nil
	}
	return l.adapter
}

// ContextTask executes a task with the canonical adapter. It is synchronous
// and may be called before Start; all QuickJS work still runs on the owner.
func (l *EventLoop) ContextTask(task func(*Context) error) error {
	if task == nil {
		return ErrNilTask
	}
	if l == nil {
		return ErrClosed
	}
	if l.isOwner() {
		if l.closed.Load() || l.state == stateClosed {
			return ErrClosed
		}
		return task(l.adapter)
	}
	return l.call(command{kind: commandContextTask, contextTask: task})
}

// DoContext executes a task synchronously while the loop is running.
func (l *EventLoop) DoContext(task func(*Context) error) error {
	if task == nil {
		return ErrNilTask
	}
	if l == nil {
		return ErrClosed
	}
	if l.isOwner() {
		if l.closed.Load() || l.state == stateClosed {
			return ErrClosed
		}
		if l.state != stateRunning {
			return ErrStopped
		}
		return task(l.adapter)
	}
	return l.call(command{kind: commandDoContextTask, contextTask: task})
}

// RunContext starts a new or stopped loop, executes the task, and drains work
// until the loop becomes idle.
func (l *EventLoop) RunContext(task func(*Context) error) error {
	if task == nil {
		return ErrNilTask
	}
	if l == nil {
		return ErrClosed
	}
	if l.isOwner() {
		return l.runContextOnOwner(task)
	}
	return l.call(command{kind: commandRunContext, contextTask: task})
}

func (l *EventLoop) runContextOnOwner(task func(*Context) error) error {
	if l.closed.Load() || l.state == stateClosed {
		return ErrClosed
	}
	if l.state == stateRunning {
		return ErrAlreadyRunning
	}
	l.state = stateRunning
	return l.runContextUntilIdle(task)
}

func (l *EventLoop) runContextUntilIdle(task func(*Context) error) error {
	if err := task(l.adapter); err != nil {
		l.state = stateStopped
		return err
	}
	for l.state == stateRunning {
		if !l.pumpOnce() {
			if !l.hasPendingWork() {
				l.state = stateStopped
				break
			}
			l.waitForWork()
		}
	}
	return nil
}

// Bind publishes a reflection-backed host object at globalThis[name]. Exported
// struct fields become accessors and exported methods become JS functions.
func (c *Context) Bind(name string, target any) error {
	if c == nil || c.loop == nil {
		return ErrClosed
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("binding name is empty")
	}
	if c.loop.isOwner() {
		return c.bindOnOwner(name, target)
	}
	return c.loop.ContextTask(func(current *Context) error {
		return current.bindOnOwner(name, target)
	})
}

func (c *Context) bindOnOwner(name string, target any) error {
	if c.raw == nil || c.loop.closed.Load() || c.loop.state == stateClosed {
		return ErrClosed
	}
	value := reflect.ValueOf(target)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return errors.New("binding target must be a non-nil pointer to a struct")
	}
	object := c.raw.NewObject()
	if object == nil {
		return errors.New("create binding object")
	}
	structValue := value.Elem()
	structType := structValue.Type()
	for index := 0; index < structType.NumField(); index++ {
		fieldType := structType.Field(index)
		if fieldType.PkgPath != "" {
			continue
		}
		propertyName, skip := bindingFieldName(fieldType)
		if skip {
			continue
		}
		field := structValue.Field(index)
		getter := c.raw.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
			result, err := ctx.Marshal(field.Interface())
			if err != nil {
				return ctx.ThrowError(err)
			}
			return result
		})
		setter := c.raw.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			if len(args) != 1 || args[0] == nil {
				return ctx.ThrowTypeError("binding field %q expects one value", propertyName)
			}
			if err := ctx.Unmarshal(args[0], field.Addr().Interface()); err != nil {
				return ctx.ThrowTypeError("binding field %q: %s", propertyName, err)
			}
			return ctx.NewUndefined()
		})
		if getter == nil || setter == nil || !object.DefinePropertyGetSet(propertyName, getter, setter, quickjs.PropConfigurable|quickjs.PropEnumerable) {
			if getter != nil {
				getter.Free()
			}
			if setter != nil {
				setter.Free()
			}
			object.Free()
			return fmt.Errorf("bind field %q", propertyName)
		}
		getter.Free()
		setter.Free()
	}
	for methodIndex := 0; methodIndex < value.NumMethod(); methodIndex++ {
		method := value.Type().Method(methodIndex)
		if method.PkgPath != "" {
			continue
		}
		propertyName := lowerCamel(method.Name)
		methodValue := value.Method(methodIndex)
		function := c.raw.NewFunction(reflectMethod(methodValue, propertyName))
		if function == nil || !object.DefinePropertyValue(propertyName, function, quickjs.PropConfigurable|quickjs.PropWritable|quickjs.PropEnumerable) {
			if function != nil {
				function.Free()
			}
			object.Free()
			return fmt.Errorf("bind method %q", propertyName)
		}
		function.Free()
	}
	c.raw.Globals().Set(name, object)
	return nil
}

func reflectMethod(method reflect.Value, name string) func(*quickjs.Context, *quickjs.Value, []*quickjs.Value) *quickjs.Value {
	methodType := method.Type()
	return func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if methodType.IsVariadic() || len(args) != methodType.NumIn() {
			return ctx.ThrowTypeError("binding method %q expects %d arguments", name, methodType.NumIn())
		}
		inputs := make([]reflect.Value, methodType.NumIn())
		for index, argument := range args {
			input := reflect.New(methodType.In(index))
			if err := ctx.Unmarshal(argument, input.Interface()); err != nil {
				return ctx.ThrowTypeError("binding method %q argument %d: %s", name, index+1, err)
			}
			inputs[index] = input.Elem()
		}
		results := method.Call(inputs)
		if len(results) > 0 {
			last := results[len(results)-1]
			if last.Type().Implements(errorType) {
				if !isNilReflectValue(last) {
					return ctx.ThrowError(last.Interface().(error))
				}
				results = results[:len(results)-1]
			}
		}
		if len(results) == 0 {
			return ctx.NewUndefined()
		}
		if len(results) == 1 {
			value, err := ctx.Marshal(results[0].Interface())
			if err != nil {
				return ctx.ThrowError(err)
			}
			return value
		}
		values := make([]any, len(results))
		for index, result := range results {
			values[index] = result.Interface()
		}
		value, err := ctx.Marshal(values)
		if err != nil {
			return ctx.ThrowError(err)
		}
		return value
	}
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

func isNilReflectValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func bindingFieldName(field reflect.StructField) (string, bool) {
	if tag, ok := field.Tag.Lookup("js"); ok {
		name := strings.Split(tag, ",")[0]
		if name == "-" {
			return "", true
		}
		if name != "" {
			return name, false
		}
	}
	if tag, ok := field.Tag.Lookup("json"); ok {
		name := strings.Split(tag, ",")[0]
		if name == "-" {
			return "", true
		}
		if name != "" {
			return name, false
		}
	}
	return lowerCamel(field.Name), false
}

func lowerCamel(name string) string {
	if name == "" {
		return ""
	}
	runes := []rune(name)
	if len(runes) > 1 && unicode.IsUpper(runes[0]) && unicode.IsUpper(runes[1]) {
		index := 0
		for index < len(runes) && unicode.IsUpper(runes[index]) {
			index++
		}
		if index > 1 && index < len(runes) {
			index--
		}
		for i := 0; i < index; i++ {
			runes[i] = unicode.ToLower(runes[i])
		}
		return string(runes)
	}
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}
