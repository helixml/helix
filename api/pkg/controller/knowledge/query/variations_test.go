package query

import "fmt"

func (suite *QuerySuite) TestCreateVariations() {
	variations, err := suite.query.createVariations(suite.ctx, "How to make HTTP call with a function?", 8)
	suite.NoError(err)

	fmt.Println(len(variations))
	for _, variation := range variations {
		fmt.Println(variation)
	}
}
