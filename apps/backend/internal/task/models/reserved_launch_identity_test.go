package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExplicitManualCreationIsBoundToRequestAndTask(t *testing.T) {
	ctx := context.Background()
	if ExplicitManualCreation(ctx, "task") {
		t.Fatal("ordinary request has manual authority")
	}
	manual := WithExplicitManualCreation(ctx, "task")
	if !ExplicitManualCreation(manual, "task") {
		t.Fatal("explicit request lost authority")
	}
	if ExplicitManualCreation(manual, "foreign") {
		t.Fatal("manual authority crossed task ownership")
	}
	if ExplicitManualCreation(WithExplicitManualCreation(manual, ""), "task") {
		t.Fatal("automatic work inherited manual authority")
	}
}

func TestReservedLaunchIdentityContextRequiresCompleteTuple(t *testing.T) {
	identity := ReservedLaunchIdentity{TaskID: "task", SessionID: "session", TurnID: "turn", SubmissionID: "submission"}
	ctx := WithReservedLaunchIdentity(context.Background(), identity)
	got, ok := ReservedLaunchIdentityFromContext(ctx)
	if !ok || got != identity {
		t.Fatalf("lost identity: %#v, %v", got, ok)
	}
	identity.SubmissionID = ""
	if _, ok := ReservedLaunchIdentityFromContext(WithReservedLaunchIdentity(ctx, identity)); ok {
		t.Fatal("incomplete identity admitted")
	}
	if _, ok := ReservedLaunchIdentityFromContext(context.Background()); ok {
		t.Fatal("identity leaked across contexts")
	}
}

func TestReservedLaunchIdentityReadDistinguishesMalformedFromMissing(t *testing.T) {
	_, exists, err := ReadReservedLaunchIdentity(context.Background())
	require.NoError(t, err)
	require.False(t, exists)
	valid := ReservedLaunchIdentity{TaskID: "task", SessionID: "session", TurnID: "turn", SubmissionID: "submission"}
	for _, field := range []string{"task", "session", "turn", "submission", "blank", "padded"} {
		t.Run(field, func(t *testing.T) {
			identity := valid
			switch field {
			case "task":
				identity.TaskID = ""
			case "session":
				identity.SessionID = ""
			case "turn":
				identity.TurnID = ""
			case "submission":
				identity.SubmissionID = ""
			case "blank":
				identity.TurnID = " "
			case "padded":
				identity.SubmissionID = " submission "
			}
			_, exists, err := ReadReservedLaunchIdentity(WithReservedLaunchIdentity(context.Background(), identity))
			require.True(t, exists)
			require.Error(t, err)
		})
	}
	identity, exists, err := ReadReservedLaunchIdentity(WithReservedLaunchIdentity(context.Background(), valid))
	require.True(t, exists)
	require.NoError(t, err)
	require.Equal(t, valid, identity)
}
