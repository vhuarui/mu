package workflows

import (
	"bytes"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/stelligent/mu/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gopkg.in/yaml.v2"
	"io"
	"testing"
)

func TestEnvironmentFinder(t *testing.T) {
	assert := assert.New(t)

	env1 := common.Environment{
		Name: "foo",
	}
	env2 := common.Environment{
		Name: "bar",
	}
	config := new(common.Config)
	config.Environments = []common.Environment{env1, env2}

	workflow := new(environmentWorkflow)

	workflow.environment = nil
	fooErr := workflow.environmentFinder(config, "foo")()
	assert.NotNil(workflow.environment)
	assert.Equal("foo", workflow.environment.Name)
	assert.Nil(fooErr)

	workflow.environment = nil
	barErr := workflow.environmentFinder(config, "bar")()
	assert.NotNil(workflow.environment)
	assert.Equal("bar", workflow.environment.Name)
	assert.Nil(barErr)

	workflow.environment = nil
	bazErr := workflow.environmentFinder(config, "baz")()
	assert.Nil(workflow.environment)
	assert.NotNil(bazErr)
}

func TestNewEnvironmentUpserter(t *testing.T) {
	assert := assert.New(t)
	ctx := common.NewContext()
	upserter := NewEnvironmentUpserter(ctx, "foo")
	assert.NotNil(upserter)
}

func TestNewEnvironmentViewer(t *testing.T) {
	assert := assert.New(t)
	ctx := common.NewContext()
	viewer := NewEnvironmentViewer(ctx, "foo", nil)
	assert.NotNil(viewer)
}

func TestNewEnvironmentLister(t *testing.T) {
	assert := assert.New(t)
	ctx := common.NewContext()
	lister := NewEnvironmentLister(ctx, nil)
	assert.NotNil(lister)
}

func TestNewEnvironmentTerminator(t *testing.T) {
	assert := assert.New(t)
	ctx := common.NewContext()
	terminator := NewEnvironmentTerminator(ctx, "foo")
	assert.NotNil(terminator)
}

type mockedStackManager struct {
	mock.Mock
}

func (m *mockedStackManager) AwaitFinalStatus(stackName string) string {
	args := m.Called(stackName)
	return args.String(0)
}
func (m *mockedStackManager) UpsertStack(stackName string, templateBodyReader io.Reader, stackParameters map[string]string, stackTags map[string]string) error {
	args := m.Called(stackName)
	return args.Error(0)
}
func (m *mockedStackManager) DeleteStack(stackName string) error {
	args := m.Called(stackName)
	return args.Error(0)
}
func (m *mockedStackManager) FindLatestImageID(pattern string) (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestEnvironmentEcsUpserter(t *testing.T) {
	assert := assert.New(t)

	workflow := new(environmentWorkflow)
	workflow.environment = &common.Environment{
		Name: "foo",
	}

	vpcInputParams := make(map[string]string)

	stackManager := new(mockedStackManager)
	stackManager.On("AwaitFinalStatus", "mu-cluster-foo").Return(cloudformation.StackStatusCreateComplete)
	stackManager.On("UpsertStack", "mu-cluster-foo").Return(nil)
	stackManager.On("FindLatestImageID").Return("ami-00000", nil)

	err := workflow.environmentEcsUpserter(vpcInputParams, stackManager, stackManager, stackManager)()
	assert.Nil(err)

	stackManager.AssertExpectations(t)
	stackManager.AssertNumberOfCalls(t, "AwaitFinalStatus", 1)
	stackManager.AssertNumberOfCalls(t, "UpsertStack", 1)
}

func TestEnvironmentVpcUpserter(t *testing.T) {
	assert := assert.New(t)

	workflow := new(environmentWorkflow)
	workflow.environment = &common.Environment{
		Name: "foo",
	}

	vpcInputParams := make(map[string]string)

	stackManager := new(mockedStackManager)
	stackManager.On("AwaitFinalStatus", "mu-vpc-foo").Return(cloudformation.StackStatusCreateComplete)
	stackManager.On("UpsertStack", "mu-vpc-foo").Return(nil)

	err := workflow.environmentVpcUpserter(vpcInputParams, stackManager, stackManager)()
	assert.Nil(err)
	assert.Equal("mu-vpc-foo-VpcId", vpcInputParams["VpcId"])
	assert.Equal("mu-vpc-foo-PublicSubnetAZ1Id", vpcInputParams["PublicSubnetAZ1Id"])
	assert.Equal("mu-vpc-foo-PublicSubnetAZ2Id", vpcInputParams["PublicSubnetAZ2Id"])
	assert.Equal("mu-vpc-foo-PublicSubnetAZ3Id", vpcInputParams["PublicSubnetAZ3Id"])

	stackManager.AssertExpectations(t)
	stackManager.AssertNumberOfCalls(t, "AwaitFinalStatus", 1)
	stackManager.AssertNumberOfCalls(t, "UpsertStack", 1)
}

func TestEnvironmentVpcUpserter_Unmanaged(t *testing.T) {
	assert := assert.New(t)
	yamlConfig :=
		`
---
environments:
  - name: dev
    vpcTarget:
      vpcId: myVpcId
      publicSubnetIds:
        - mySubnetId1
        - mySubnetId2
`
	config, err := loadYamlConfig(yamlConfig)
	assert.Nil(err)

	vpcInputParams := make(map[string]string)

	stackManager := new(mockedStackManager)

	workflow := new(environmentWorkflow)
	workflow.environment = &config.Environments[0]

	err = workflow.environmentVpcUpserter(vpcInputParams, stackManager, stackManager)()
	assert.Nil(err)
	assert.Equal("myVpcId", vpcInputParams["VpcId"])
	assert.Equal("mySubnetId1", vpcInputParams["PublicSubnetAZ1Id"])
	assert.Equal("mySubnetId2", vpcInputParams["PublicSubnetAZ2Id"])

	stackManager.AssertExpectations(t)
	stackManager.AssertNumberOfCalls(t, "AwaitFinalStatus", 0)
	stackManager.AssertNumberOfCalls(t, "UpsertStack", 0)
}

func loadYamlConfig(yamlString string) (*common.Config, error) {
	config := new(common.Config)
	yamlBuffer := new(bytes.Buffer)
	yamlBuffer.ReadFrom(bytes.NewBufferString(yamlString))
	err := yaml.Unmarshal(yamlBuffer.Bytes(), config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func TestNewEnvironmentEcsTerminator(t *testing.T) {
	assert := assert.New(t)

	workflow := new(environmentWorkflow)
	workflow.environment = &common.Environment{
		Name: "foo",
	}

	stackManager := new(mockedStackManager)
	stackManager.On("AwaitFinalStatus", "mu-cluster-foo").Return(cloudformation.StackStatusDeleteComplete)
	stackManager.On("DeleteStack", "mu-cluster-foo").Return(nil)

	err := workflow.environmentEcsTerminator("foo", stackManager, stackManager)()
	assert.Nil(err)

	stackManager.AssertExpectations(t)
	stackManager.AssertNumberOfCalls(t, "AwaitFinalStatus", 1)
	stackManager.AssertNumberOfCalls(t, "DeleteStack", 1)
}

func TestNewEnvironmentVpcTerminator(t *testing.T) {
	assert := assert.New(t)

	workflow := new(environmentWorkflow)
	workflow.environment = &common.Environment{
		Name: "foo",
	}

	stackManager := new(mockedStackManager)
	stackManager.On("AwaitFinalStatus", "mu-vpc-foo").Return(cloudformation.StackStatusDeleteComplete)
	stackManager.On("DeleteStack", "mu-vpc-foo").Return(nil)

	err := workflow.environmentVpcTerminator("foo", stackManager, stackManager)()
	assert.Nil(err)

	stackManager.AssertExpectations(t)
	stackManager.AssertNumberOfCalls(t, "AwaitFinalStatus", 1)
	stackManager.AssertNumberOfCalls(t, "DeleteStack", 1)
}