package phpx

import "testing"

func TestGenerator(t *testing.T) {
	counter := NewGenerator(func(y *Yielder) any {
		total := 0
		for i := 1; i <= 3; i++ {
			got := y.Yield(nil, i)
			total += ToInt(got)
		}
		return total
	})
	var seen []int
	for counter.Valid() {
		seen = append(seen, ToInt(counter.Current()))
		counter.Send(10)
	}
	if len(seen) != 3 || seen[2] != 3 || ToInt(counter.GetReturn()) != 30 {
		t.Fatalf("seen %v, return %v", seen, counter.GetReturn())
	}

	// throw() into a generator that catches it, and yield from.
	inner := func() *Generator {
		return NewGenerator(func(y *Yielder) any {
			y.Yield("a", 1)
			return "inner done"
		})
	}
	outer := NewGenerator(func(y *Yielder) any {
		r := y.YieldFrom(inner())
		_, v := Try(func() (int, any) {
			y.Yield(nil, "waiting")
			return 0, nil
		}, nil, Catch{Classes: []string{"exception"}, Body: func(e Throwable) (int, any) {
			return 1, "caught " + e.GetMessage()
		}})
		return ToString(r) + ", " + ToString(v)
	})
	if outer.Key() != "a" || ToInt(outer.Current()) != 1 {
		t.Fatalf("first yield: %v => %v", outer.Key(), outer.Current())
	}
	outer.Next()
	if outer.Current() != "waiting" {
		t.Fatalf("second yield: %v", outer.Current())
	}
	outer.Throw(NewException("Exception", "boom"))
	if outer.Valid() || outer.GetReturn() != "inner done, caught boom" {
		t.Fatalf("return: %v", outer.GetReturn())
	}

	// An uncaught exception reaches the caller.
	bad := NewGenerator(func(y *Yielder) any {
		Throw(NewException("RuntimeException", "bad"))
		return nil
	})
	ctl, val := Try(func() (int, any) { bad.Current(); return 0, nil }, nil,
		Catch{Classes: []string{"runtimeexception"}, Body: func(e Throwable) (int, any) { return 1, e.GetMessage() }})
	if ctl != 1 || val != "bad" {
		t.Fatalf("exception: %v %v", ctl, val)
	}
}
