// generated by stringer -type State; DO NOT EDIT

package miyabi

import "fmt"

const _State_name = "StateStartStateRestartStateShutdown"

var _State_index = [...]uint8{10, 22, 35}

func (i State) String() string {
	if i >= State(len(_State_index)) {
		return fmt.Sprintf("State(%d)", i)
	}
	hi := _State_index[i]
	lo := uint8(0)
	if i > 0 {
		lo = _State_index[i-1]
	}
	return _State_name[lo:hi]
}
