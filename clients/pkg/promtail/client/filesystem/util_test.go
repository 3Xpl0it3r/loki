package filesystem

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDeploymentName(t *testing.T){
	controllerNameList := []string{
		"lts-jobtracker-698d6c7988",
		"lts-698d6c7988",
		"dmo-pre-workf-inf-tag1-deploy-64956bcdf",
	}
	deploymentNameList := []string{
		"lts-jobtracker",
		"lts",
		"dmo-pre-workf-inf-tag1-deploy",
	}
	for index, name := range controllerNameList{
		assert.Equal(t, deploymentNameList[index], DeploymentName(name))
	}
}

