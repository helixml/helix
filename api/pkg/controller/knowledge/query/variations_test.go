package query

import "fmt"

func (suite *QuerySuite) TestCreateVariations() {
	variations, err := suite.query.createVariations(suite.ctx, "How to make HTTP call with a function?", 8)
	suite.NoError(err)

	suite.Equal(8, len(variations))
	// TODO: use an LLM to check if the variations are valid

	fmt.Println(len(variations))
	for _, variation := range variations {
		fmt.Println(variation)
	}
}
