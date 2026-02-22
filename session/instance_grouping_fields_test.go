package session

import (
	"reflect"
	"testing"
)

func TestPlanGroupingModel_HasNoLegacyTopicFields(t *testing.T) {
	if _, ok := reflect.TypeOf(Instance{}).FieldByName("TopicName"); ok {
		t.Fatalf("Instance still has legacy TopicName field")
	}
	if _, ok := reflect.TypeOf(InstanceOptions{}).FieldByName("TopicName"); ok {
		t.Fatalf("InstanceOptions still has legacy TopicName field")
	}
	if _, ok := reflect.TypeOf(InstanceData{}).FieldByName("TopicName"); ok {
		t.Fatalf("InstanceData still has legacy TopicName field")
	}

	if _, ok := reflect.TypeOf(Instance{}).FieldByName("PlanFile"); !ok {
		t.Fatalf("Instance must include PlanFile")
	}
}
