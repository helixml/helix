package types

var (
	// Read role can see and access most of the resources
	RoleRead = Config{
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
				},
				Effect: EffectAllow,
			},
		},
	}

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
		Name:   "read",
		Config: RoleRead,
	},
	{
		Name:   "write",
		Config: RoleWrite,
	},
	{
		Name:   "admin",
		Config: RoleAdmin,
	},
}
