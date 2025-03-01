package types

var (
	// Read role can see and access most of the resources
	RoleRead = Config{
		Rules: []Rule{
			{
				Resources: []Resource{
					ResourceApplication,
					ResourceKnowledge,
					ResourceAccessGrants,
				},
				Actions: []Action{
					ActionGet,
					ActionList,
					ActionUseAction,
				},
				Effect: EffectAllow,
			},
		},
	}

	// Can update applications, knowledge, however no
	// access grants can be updated
	RoleWrite = Config{
		Rules: []Rule{
			{
				Resources: []Resource{
					ResourceApplication,
					ResourceKnowledge,
				},
				Actions: []Action{
					ActionGet,
					ActionList,
					ActionUseAction,
					ActionCreate,
					ActionUpdate,
					ActionDelete,
				},
				Effect: EffectAllow,
			},
		},
	}

	RoleAdmin = Config{
		Rules: []Rule{
			{
				Resources: []Resource{
					ResourceAny,
				},
				Actions: []Action{
					ActionGet,
					ActionList,
					ActionUseAction,
					ActionCreate,
					ActionUpdate,
					ActionDelete,
				},
				Effect: EffectAllow,
			},
		},
	}
)

var Roles = []Role{
	{
		Name:        "read",
		Config:      RoleRead,
		Description: "Can view applications, knowledge, and run applications. Cannot edit configuration",
	},
	{
		Name:        "write",
		Config:      RoleWrite,
		Description: "Can view applications, knowledge, and run actions, create, update, and delete applications and knowledge",
	},
	{
		Name:        "admin",
		Config:      RoleAdmin,
		Description: "Can perform all actions including updating access grants",
	},
}
