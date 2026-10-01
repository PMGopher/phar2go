package phpx

import "testing"

type serNode struct {
	name string
	next *serNode
	self any
	data *Array
}

func TestSerializeObject(t *testing.T) {
	n := &serNode{name: "a", data: List(1, 2)}
	n.next = &serNode{name: "b", next: n}
	n.self = n
	s := Serialize(n)
	n.name = "changed"
	n.data.Set(0, 9)
	got := Unserialize(s).(*serNode)
	if got == n || got.name != "a" || got.next.name != "b" || got.next.next != got || got.self != any(got) || got.data.Get(0) != 1 {
		t.Fatalf("bad copy: %+v", got)
	}
	if Serialize(List(1, "x")) != `[1,"x"]` {
		t.Fatal(Serialize(List(1, "x")))
	}
}
