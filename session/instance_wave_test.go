package session

import (
	"testing"
)

func TestNewInstance_SetsWaveAndTaskNumber(t *testing.T) {
	inst, err := NewInstance(InstanceOptions{
		Title:      "test-T1",
		Path:       "/tmp",
		Program:    "echo",
		PlanFile:   "plan.md",
		TaskNumber: 1,
		WaveNumber: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inst.TaskNumber != 1 {
		t.Fatalf("TaskNumber = %d, want 1", inst.TaskNumber)
	}
	if inst.WaveNumber != 1 {
		t.Fatalf("WaveNumber = %d, want 1", inst.WaveNumber)
	}
}

func TestInstanceData_RoundTripWaveFields(t *testing.T) {
	inst, _ := NewInstance(InstanceOptions{
		Title:      "test-T2",
		Path:       "/tmp",
		Program:    "echo",
		PlanFile:   "plan.md",
		TaskNumber: 3,
		WaveNumber: 2,
	})

	data := inst.ToInstanceData()
	if data.TaskNumber != 3 {
		t.Fatalf("InstanceData TaskNumber = %d, want 3", data.TaskNumber)
	}
	if data.WaveNumber != 2 {
		t.Fatalf("InstanceData WaveNumber = %d, want 2", data.WaveNumber)
	}

	restored, err := FromInstanceData(data)
	if err != nil {
		t.Fatal(err)
	}
	if restored.TaskNumber != 3 {
		t.Fatalf("restored TaskNumber = %d, want 3", restored.TaskNumber)
	}
	if restored.WaveNumber != 2 {
		t.Fatalf("restored WaveNumber = %d, want 2", restored.WaveNumber)
	}
}
