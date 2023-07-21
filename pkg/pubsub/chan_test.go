package pubsub

import "testing"

func TestPubSubChan(t *testing.T) {
	testPubSub(t, NewPubSubChan[int]())
}

func testPubSub(t *testing.T, ps PubSub[int]) {
	s1 := ps.Subscribe()
	s2 := ps.Subscribe()

	num := 100

	for i := 1; i <= num; i++ {
		if err := ps.Publish(i); err != nil {
			t.Fatal(err)
		}
	}

	for _, s := range [](<-chan Result[int]){s2, s1} {
		for i := 1; i <= num; i++ {
			r := <-s
			if r.Err != nil {
				t.Fatal(r.Err)
			}
			if r.Ok != i {
				t.Fatalf("expected %d, got %d", i, r.Ok)
			}
		}
	}
}
