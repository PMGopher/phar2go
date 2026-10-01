package phpx

// Generator is PHP's Generator: a function that yields values. The body runs on its own
// goroutine, but only one side runs at a time (the caller waits while the body runs and the
// other way around), like PHP's generators run on the caller's thread.
type Generator struct {
	body     func(y *Yielder) any
	started  bool
	finished bool
	key      any
	current  any
	ret      any
	autoKey  int

	resume chan resumeMsg
	yields chan yieldMsg
}

type resumeMsg struct {
	val any
	exc any
}

type yieldMsg struct {
	key, val any
	done     bool
	ret      any
	panicked bool
	panicVal any
}

// Yielder is how a generator body yields.
type Yielder struct{ g *Generator }

// NewGenerator creates a generator from its body. The body's return value is GetReturn().
func NewGenerator(body func(y *Yielder) any) *Generator {
	return &Generator{body: body}
}

func (g *Generator) start() {
	if g.started {
		return
	}
	g.started = true
	g.resume = make(chan resumeMsg)
	g.yields = make(chan yieldMsg)
	go func() {
		var msg yieldMsg
		defer func() {
			if r := recover(); r != nil {
				msg = yieldMsg{done: true, panicked: true, panicVal: r}
			}
			g.yields <- msg
		}()
		// Wait for the first resume (the caller's first use of the generator).
		first := <-g.resume
		if first.exc != nil {
			panic(first.exc)
		}
		ret := g.body(&Yielder{g})
		msg = yieldMsg{done: true, ret: ret}
	}()
	g.step(resumeMsg{})
}

// step resumes the body and waits for its next yield (or its end).
func (g *Generator) step(r resumeMsg) {
	g.resume <- r
	msg := <-g.yields
	if msg.done {
		g.finished = true
		g.key, g.current = nil, nil
		g.ret = msg.ret
		if msg.panicked {
			panic(msg.panicVal)
		}
		return
	}
	g.key, g.current = msg.key, msg.val
}

// Yield is `yield $key => $value` (key nil: the next integer key). It returns the value sent
// with Send(), or null.
func (y *Yielder) Yield(key, val any) any {
	g := y.g
	if key == nil {
		key = g.autoKey
		g.autoKey++
	} else if n, ok := key.(int); ok && n >= g.autoKey {
		g.autoKey = n + 1
	}
	g.yields <- yieldMsg{key: key, val: val}
	r := <-g.resume
	if r.exc != nil {
		panic(r.exc)
	}
	return r.val
}

// YieldFrom is `yield from $inner`: it yields everything inner yields and returns inner's
// return value. inner may be a generator or an array.
func (y *Yielder) YieldFrom(inner any) any {
	gen, ok := inner.(*Generator)
	if !ok {
		for _, e := range Iter(inner) {
			y.Yield(e.Key, e.Val)
		}
		return nil
	}
	gen.start()
	for !gen.finished {
		y.g.yields <- yieldMsg{key: gen.key, val: gen.current}
		r := <-y.g.resume
		if r.exc != nil {
			gen.step(resumeMsg{exc: r.exc})
		} else {
			gen.step(resumeMsg{val: r.val})
		}
	}
	return gen.ret
}

// Current is Generator::current().
func (g *Generator) Current() any {
	g.start()
	return g.current
}

// Key is Generator::key().
func (g *Generator) Key() any {
	g.start()
	return g.key
}

// Next is Generator::next().
func (g *Generator) Next() {
	g.start()
	if !g.finished {
		g.step(resumeMsg{})
	}
}

// Send is Generator::send(): it resumes the generator with v as the result of the current
// yield and returns the next yielded value.
func (g *Generator) Send(v any) any {
	if !g.started {
		g.start()
	}
	if g.finished {
		return nil
	}
	g.step(resumeMsg{val: v})
	return g.current
}

// Throw is Generator::throw(): the exception is thrown where the generator is suspended.
func (g *Generator) Throw(e any) any {
	if !g.started {
		g.start()
	}
	if g.finished {
		Throw(e)
	}
	g.step(resumeMsg{exc: AsThrowable(e)})
	return g.current
}

// Valid is Generator::valid().
func (g *Generator) Valid() bool {
	g.start()
	return !g.finished
}

// Rewind is Generator::rewind().
func (g *Generator) Rewind() { g.start() }

// GetReturn is Generator::getReturn().
func (g *Generator) GetReturn() any {
	if !g.finished {
		Throw(NewException("Exception", "Cannot get return value of a generator that hasn't returned"))
	}
	return g.ret
}

// PhpIterate drains the generator for foreach over an untyped value.
func (g *Generator) PhpIterate() []Entry {
	var out []Entry
	for g.Valid() {
		out = append(out, Entry{g.Key(), g.Current()})
		g.Next()
	}
	return out
}

// PhpClass is the PHP class name.
func (g *Generator) PhpClass() string { return "Generator" }
