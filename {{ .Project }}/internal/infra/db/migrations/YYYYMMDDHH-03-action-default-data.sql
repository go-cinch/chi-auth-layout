-- +migrate Up
-- Final action definitions. Codes remain stable assignment identities; words and
-- multilingual action keys identify each scope:operation:module:resource.
INSERT INTO t_action (id, action_group, code, word, config) VALUES
    (1, 'admin:system:high-risk', 'M6TG8SR9', 'admin:all:system:all', '{
    "ui": [],
    "bindings": [
        {
            "operation": "*",
            "feature_keys": [],
            "resource_key": "admin:high-risk:all"
        }
    ],
    "endpoints": [],
    "feature_keys": null,
    "permission_scope": "*"
}'::JSONB),
    (2, 'admin:system:default', 'B7RP4HYW', 'admin:default:system:default', '{
    "ui": [
        {
            "menu_key": "admin:menu:account:profile",
            "menu_path": "/profile",
            "page_entry": true,
            "button_keys": [
                "admin:btn:account:profile:change-password"
            ]
        },
        {
            "menu_key": "admin:menu:msg:inbox",
            "menu_path": "/msg/inbox",
            "page_entry": true,
            "button_keys": [
                "admin:btn:msg:inbox:query",
                "admin:btn:msg:inbox:refresh",
                "admin:btn:msg:inbox:view-detail",
                "admin:btn:msg:inbox:mark-read",
                "admin:btn:msg:inbox:mark-selected-read",
                "admin:btn:msg:inbox:read-all",
                "admin:btn:msg:inbox:delete",
                "admin:btn:msg:inbox:delete-selected"
            ]
        },
        {
            "menu_key": "admin:menu:dashboard:overview",
            "menu_path": "/dashboard/overview",
            "page_entry": true,
            "button_keys": [],
            "parent_menu_key": "admin:menu:dashboard"
        }
    ],
    "builtin": "default",
    "endpoints": [
        {
            "path": "/auth/change/pwd",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/auth/captcha",
            "method": "POST",
            "protocol": "http"
        },
        {
            "path": "/auth/captcha/verify",
            "method": "POST",
            "protocol": "http"
        },
        {
            "path": "/msg/inbox/{id}",
            "method": "DELETE",
            "protocol": "http"
        },
        {
            "path": "/msg/inbox",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/msg/inbox/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/msg/unread-count",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/msg/inbox/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/msg/inbox/read-all",
            "method": "POST",
            "protocol": "http"
        },
        {
            "path": "/auth/info",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/auth/reset/pwd",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/auth/challenge",
            "method": "POST",
            "protocol": "http"
        }
    ],
    "feature_keys": [
        "admin:account:profile:default-access",
        "admin:msg:inbox:default-access",
        "admin:default:session:access"
    ],
    "resource_keys": [
        "admin:account:profile",
        "admin:msg:inbox",
        "admin:default:session",
        "admin:dashboard:overview"
    ]
}'::JSONB),
    (3, 'admin:dashboard:overview', '35VY9TRL', 'admin:read:dashboard:overview', '{
    "ui": [
        {
            "menu_key": "admin:menu:dashboard:overview",
            "menu_path": "/dashboard/overview",
            "page_entry": true,
            "button_keys": [],
            "parent_menu_key": "admin:menu:dashboard"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [],
            "resource_key": "admin:dashboard:overview"
        }
    ],
    "endpoints": []
}'::JSONB),
    (4, 'admin:system:user', 'Y6RW5544', 'admin:read:system:user', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:user",
            "menu_path": "/system/user",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:user:query",
                "admin:btn:system:user:reset-query",
                "admin:btn:system:user:refresh",
                "admin:btn:system:user:view-detail"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:user:query",
                "admin:system:user:reset-query",
                "admin:system:user:refresh",
                "admin:system:user:view-detail"
            ],
            "resource_key": "admin:system:user"
        }
    ],
    "endpoints": [
        {
            "path": "/user",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user/{id}",
            "method": "GET",
            "protocol": "http"
        }
    ],
    "feature_keys": null
}'::JSONB),
    (5, 'admin:system:user', 'PMWSQYY5', 'admin:create:system:user', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:user",
            "menu_path": "/system/user",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user:create"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:user:create"
            ],
            "resource_key": "admin:system:user"
        }
    ],
    "endpoints": [
        {
            "path": "/user",
            "method": "POST",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (6, 'admin:system:user', 'R2LTBEEG', 'admin:update:system:user', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:user",
            "menu_path": "/system/user",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user:edit",
                "admin:btn:system:user:approve",
                "admin:btn:system:user:lock",
                "admin:btn:system:user:unlock",
                "admin:btn:system:user:unlock-password"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:user:edit",
                "admin:system:user:approve",
                "admin:system:user:lock",
                "admin:system:user:unlock",
                "admin:system:user:unlock-password"
            ],
            "resource_key": "admin:system:user"
        }
    ],
    "endpoints": [
        {
            "path": "/user/{id}",
            "method": "PATCH",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (7, 'admin:system:user', 'JBVF9YWV', 'admin:delete:system:user', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:user",
            "menu_path": "/system/user",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user:delete",
                "admin:btn:system:user:delete-selected"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:user:delete",
                "admin:system:user:delete-selected"
            ],
            "resource_key": "admin:system:user"
        }
    ],
    "endpoints": [
        {
            "path": "/user/{id}",
            "method": "DELETE",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (8, 'admin:system:user-group', 'F9PEAFVW', 'admin:read:system:user-group', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:user-group",
            "menu_path": "/system/user-group",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:user-group:query",
                "admin:btn:system:user-group:reset-query",
                "admin:btn:system:user-group:refresh",
                "admin:btn:system:user-group:view-detail",
                "admin:btn:system:user-group:view-members"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:user-group:query",
                "admin:system:user-group:reset-query",
                "admin:system:user-group:refresh",
                "admin:system:user-group:view-detail",
                "admin:system:user-group:view-members"
            ],
            "resource_key": "admin:system:user-group"
        }
    ],
    "endpoints": [
        {
            "path": "/user-group",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user-group/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user-group/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user-group/{id}/member",
            "method": "GET",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (9, 'admin:system:user-group', 'VLN9HX2E', 'admin:create:system:user-group', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:user-group",
            "menu_path": "/system/user-group",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user-group:create"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:user-group:create"
            ],
            "resource_key": "admin:system:user-group"
        }
    ],
    "endpoints": [
        {
            "path": "/user-group",
            "method": "POST",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (10, 'admin:system:user-group', 'K4AQMSC6', 'admin:update:system:user-group', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:user-group",
            "menu_path": "/system/user-group",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user-group:edit",
                "admin:btn:system:user-group:add-members",
                "admin:btn:system:user-group:remove-members"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:user-group:edit",
                "admin:system:user-group:add-members",
                "admin:system:user-group:remove-members"
            ],
            "resource_key": "admin:system:user-group"
        }
    ],
    "endpoints": [
        {
            "path": "/user-group/{id}",
            "method": "PATCH",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (11, 'admin:system:user-group', 'B2VYAELW', 'admin:delete:system:user-group', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:user-group",
            "menu_path": "/system/user-group",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user-group:delete",
                "admin:btn:system:user-group:delete-selected"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:user-group:delete",
                "admin:system:user-group:delete-selected"
            ],
            "resource_key": "admin:system:user-group"
        }
    ],
    "endpoints": [
        {
            "path": "/user-group/{id}",
            "method": "DELETE",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (12, 'admin:system:role', 'XXY567KR', 'admin:read:system:role', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:role",
            "menu_path": "/system/role",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:role:query",
                "admin:btn:system:role:reset-query",
                "admin:btn:system:role:refresh",
                "admin:btn:system:role:view-detail"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:role:query",
                "admin:system:role:reset-query",
                "admin:system:role:refresh",
                "admin:system:role:view-detail"
            ],
            "resource_key": "admin:system:role"
        }
    ],
    "endpoints": [
        {
            "path": "/role",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/role/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/role/{id}",
            "method": "GET",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (13, 'admin:system:role', '3V5GWBQT', 'admin:create:system:role', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:role",
            "menu_path": "/system/role",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:role:create"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:role:create"
            ],
            "resource_key": "admin:system:role"
        }
    ],
    "endpoints": [
        {
            "path": "/role",
            "method": "POST",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (14, 'admin:system:role', 'QYALLJEG', 'admin:update:system:role', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:role",
            "menu_path": "/system/role",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:role:edit"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:role:edit"
            ],
            "resource_key": "admin:system:role"
        }
    ],
    "endpoints": [
        {
            "path": "/role/{id}",
            "method": "PATCH",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (15, 'admin:system:role', 'K5VF772F', 'admin:delete:system:role', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:role",
            "menu_path": "/system/role",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:role:delete",
                "admin:btn:system:role:delete-selected"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:role:delete",
                "admin:system:role:delete-selected"
            ],
            "resource_key": "admin:system:role"
        }
    ],
    "endpoints": [
        {
            "path": "/role/{id}",
            "method": "DELETE",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (16, 'admin:system:action', 'E66VGBKA', 'admin:read:system:action', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:action",
            "menu_path": "/system/action",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:action:query",
                "admin:btn:system:action:reset-query",
                "admin:btn:system:action:refresh",
                "admin:btn:system:action:view-detail"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:action:query",
                "admin:system:action:reset-query",
                "admin:system:action:refresh",
                "admin:system:action:view-detail"
            ],
            "resource_key": "admin:system:action"
        }
    ],
    "endpoints": [
        {
            "path": "/action",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/catalog",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/group",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/{id}",
            "method": "GET",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (17, 'admin:system:action', '5RNWYB4S', 'admin:create:system:action', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:action",
            "menu_path": "/system/action",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:action:create",
                "admin:btn:system:action:create-group"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:action:create",
                "admin:system:action:create-group"
            ],
            "resource_key": "admin:system:action"
        }
    ],
    "endpoints": [
        {
            "path": "/dictionary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/dictionary/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/action",
            "method": "POST",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (18, 'admin:system:action', 'TW535HD8', 'admin:update:system:action', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:action",
            "menu_path": "/system/action",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:action:edit",
                "admin:btn:system:action:reorder-groups"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:action:edit",
                "admin:system:action:reorder-groups"
            ],
            "resource_key": "admin:system:action"
        }
    ],
    "endpoints": [
        {
            "path": "/dictionary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/dictionary/{id}",
            "method": "PATCH",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (19, 'admin:system:action', 'V75R23SD', 'admin:delete:system:action', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:action",
            "menu_path": "/system/action",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:action:delete",
                "admin:btn:system:action:delete-selected",
                "admin:btn:system:action:delete-group"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:action:delete",
                "admin:system:action:delete-selected",
                "admin:system:action:delete-group"
            ],
            "resource_key": "admin:system:action"
        }
    ],
    "endpoints": [
        {
            "path": "/action/{id}",
            "method": "DELETE",
            "protocol": "http"
        },
        {
            "path": "/dictionary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/dictionary/{id}",
            "method": "PATCH",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (24, 'admin:system:dictionary', 'H3HW2LEB', 'admin:read:system:dictionary', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:dictionary",
            "menu_path": "/system/dictionary",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:dictionary:query",
                "admin:btn:system:dictionary:reset-query",
                "admin:btn:system:dictionary:refresh",
                "admin:btn:system:dictionary:view-detail"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:dictionary:query",
                "admin:system:dictionary:reset-query",
                "admin:system:dictionary:refresh",
                "admin:system:dictionary:view-detail"
            ],
            "resource_key": "admin:system:dictionary"
        }
    ],
    "endpoints": [
        {
            "path": "/dictionary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/dictionary/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/dictionary/{id}",
            "method": "GET",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (25, 'admin:system:dictionary', 'HED6QQTL', 'admin:create:system:dictionary', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:dictionary",
            "menu_path": "/system/dictionary",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:dictionary:create"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:dictionary:create"
            ],
            "resource_key": "admin:system:dictionary"
        }
    ],
    "endpoints": [
        {
            "path": "/dictionary",
            "method": "POST",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (26, 'admin:system:dictionary', 'E3WB6LE7', 'admin:update:system:dictionary', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:dictionary",
            "menu_path": "/system/dictionary",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:dictionary:edit"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:dictionary:edit"
            ],
            "resource_key": "admin:system:dictionary"
        }
    ],
    "endpoints": [
        {
            "path": "/dictionary/{id}",
            "method": "PATCH",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (27, 'admin:system:dictionary', 'MCHJ3RY3', 'admin:delete:system:dictionary', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:dictionary",
            "menu_path": "/system/dictionary",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:dictionary:delete",
                "admin:btn:system:dictionary:delete-selected"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:dictionary:delete",
                "admin:system:dictionary:delete-selected"
            ],
            "resource_key": "admin:system:dictionary"
        }
    ],
    "endpoints": [
        {
            "path": "/dictionary/{id}",
            "method": "DELETE",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (28, 'admin:system:msg', 'JJNWC246', 'admin:read:system:msg', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:msg",
            "menu_path": "/system/msg",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:msg:query",
                "admin:btn:system:msg:reset-query",
                "admin:btn:system:msg:refresh",
                "admin:btn:system:msg:view-detail"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:msg:query",
                "admin:system:msg:reset-query",
                "admin:system:msg:refresh",
                "admin:system:msg:view-detail"
            ],
            "resource_key": "admin:system:msg"
        }
    ],
    "endpoints": [
        {
            "path": "/msg/sent",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/msg/sent/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/msg/sent/{id}",
            "method": "GET",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (29, 'admin:system:msg', '9TS8JETG', 'admin:create:system:msg', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:msg",
            "menu_path": "/system/msg",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:msg:send"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:msg:send"
            ],
            "resource_key": "admin:system:msg"
        }
    ],
    "endpoints": [
        {
            "path": "/msg/recipient-option",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/msg",
            "method": "POST",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (30, 'admin:system:msg', 'SNP225K5', 'admin:delete:system:msg', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:msg",
            "menu_path": "/system/msg",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:msg:delete",
                "admin:btn:system:msg:delete-selected"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:msg:delete",
                "admin:system:msg:delete-selected"
            ],
            "resource_key": "admin:system:msg"
        }
    ],
    "endpoints": [
        {
            "path": "/msg/sent/{id}",
            "method": "DELETE",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (31, 'admin:audit:login', '47WDGP7H', 'admin:read:audit:login', '{
    "ui": [
        {
            "menu_key": "admin:menu:audit:login",
            "menu_path": "/audit/login",
            "page_entry": true,
            "button_keys": [
                "admin:btn:audit:login:query",
                "admin:btn:audit:login:reset-query",
                "admin:btn:audit:login:refresh",
                "admin:btn:audit:login:view-detail",
                "admin:btn:audit:login:export-report"
            ],
            "parent_menu_key": "admin:menu:audit"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:audit:login:query",
                "admin:audit:login:reset-query",
                "admin:audit:login:refresh",
                "admin:audit:login:view-detail",
                "admin:audit:login:export-report"
            ],
            "resource_key": "admin:audit:login"
        }
    ],
    "endpoints": [
        {
            "path": "/audit/login",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/audit/login/detail/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/audit/login/export",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/audit/login/option",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/audit/login/summary",
            "method": "GET",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (33, 'admin:audit:operation', 'GW3JY8SJ', 'admin:read:audit:operation', '{
    "ui": [
        {
            "menu_key": "admin:menu:audit:operation",
            "menu_path": "/audit/operation",
            "page_entry": true,
            "button_keys": [
                "admin:btn:audit:operation:query",
                "admin:btn:audit:operation:reset-query",
                "admin:btn:audit:operation:refresh",
                "admin:btn:audit:operation:view-detail",
                "admin:btn:audit:operation:export-report"
            ],
            "parent_menu_key": "admin:menu:audit"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:audit:operation:query",
                "admin:audit:operation:reset-query",
                "admin:audit:operation:refresh",
                "admin:audit:operation:view-detail",
                "admin:audit:operation:export-report"
            ],
            "resource_key": "admin:audit:operation"
        }
    ],
    "endpoints": [
        {
            "path": "/audit/operation",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/audit/operation/detail/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/audit/operation/export",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/audit/operation/option",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/audit/operation/summary",
            "method": "GET",
            "protocol": "http"
        }
    ]
}'::JSONB),
    (35, 'admin:dashboard:workspace', 'SKFRSH2E', 'admin:read:dashboard:workspace', '{
    "ui": [
        {
            "menu_key": "admin:menu:dashboard:workspace",
            "menu_path": "/dashboard/workspace",
            "page_entry": true,
            "button_keys": [],
            "parent_menu_key": "admin:menu:dashboard"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [],
            "resource_key": "admin:dashboard:workspace"
        }
    ],
    "endpoints": []
}'::JSONB),
    (36, 'admin:system:high-risk', 'HFSNA574', 'admin:grant:system:permission-admin', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:role",
            "menu_path": "/system/role",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:role:query",
                "admin:btn:system:role:reset-query",
                "admin:btn:system:role:refresh",
                "admin:btn:system:role:view-detail",
                "admin:btn:system:role:create",
                "admin:btn:system:role:edit",
                "admin:btn:system:role:delete",
                "admin:btn:system:role:delete-selected"
            ],
            "parent_menu_key": "admin:menu:system"
        },
        {
            "menu_key": "admin:menu:system:action",
            "menu_path": "/system/action",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:action:query",
                "admin:btn:system:action:reset-query",
                "admin:btn:system:action:refresh",
                "admin:btn:system:action:view-detail",
                "admin:btn:system:action:create",
                "admin:btn:system:action:create-group",
                "admin:btn:system:action:edit",
                "admin:btn:system:action:reorder-groups",
                "admin:btn:system:action:delete",
                "admin:btn:system:action:delete-selected",
                "admin:btn:system:action:delete-group"
            ],
            "parent_menu_key": "admin:menu:system"
        },
        {
            "menu_key": "admin:menu:system:user",
            "menu_path": "/system/user",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:user:query",
                "admin:btn:system:user:reset-query",
                "admin:btn:system:user:refresh",
                "admin:btn:system:user:view-detail",
                "admin:btn:system:user:create",
                "admin:btn:system:user:edit",
                "admin:btn:system:user:approve",
                "admin:btn:system:user:lock",
                "admin:btn:system:user:unlock",
                "admin:btn:system:user:unlock-password",
                "admin:btn:system:user:delete",
                "admin:btn:system:user:delete-selected"
            ],
            "parent_menu_key": "admin:menu:system"
        },
        {
            "menu_key": "admin:menu:system:user-group",
            "menu_path": "/system/user-group",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:user-group:query",
                "admin:btn:system:user-group:reset-query",
                "admin:btn:system:user-group:refresh",
                "admin:btn:system:user-group:view-detail",
                "admin:btn:system:user-group:view-members",
                "admin:btn:system:user-group:create",
                "admin:btn:system:user-group:edit",
                "admin:btn:system:user-group:add-members",
                "admin:btn:system:user-group:remove-members",
                "admin:btn:system:user-group:delete",
                "admin:btn:system:user-group:delete-selected"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:role:query",
                "admin:system:role:reset-query",
                "admin:system:role:refresh",
                "admin:system:role:view-detail"
            ],
            "resource_key": "admin:system:role"
        },
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:role:create"
            ],
            "resource_key": "admin:system:role"
        },
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:role:edit"
            ],
            "resource_key": "admin:system:role"
        },
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:role:delete",
                "admin:system:role:delete-selected"
            ],
            "resource_key": "admin:system:role"
        },
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:action:query",
                "admin:system:action:reset-query",
                "admin:system:action:refresh",
                "admin:system:action:view-detail"
            ],
            "resource_key": "admin:system:action"
        },
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:action:create",
                "admin:system:action:create-group"
            ],
            "resource_key": "admin:system:action"
        },
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:action:edit",
                "admin:system:action:reorder-groups"
            ],
            "resource_key": "admin:system:action"
        },
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:action:delete",
                "admin:system:action:delete-selected",
                "admin:system:action:delete-group"
            ],
            "resource_key": "admin:system:action"
        },
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:user:query",
                "admin:system:user:reset-query",
                "admin:system:user:refresh",
                "admin:system:user:view-detail"
            ],
            "resource_key": "admin:system:user"
        },
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:user:create"
            ],
            "resource_key": "admin:system:user"
        },
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:user:edit",
                "admin:system:user:approve",
                "admin:system:user:lock",
                "admin:system:user:unlock",
                "admin:system:user:unlock-password"
            ],
            "resource_key": "admin:system:user"
        },
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:user:delete",
                "admin:system:user:delete-selected"
            ],
            "resource_key": "admin:system:user"
        },
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:user-group:query",
                "admin:system:user-group:reset-query",
                "admin:system:user-group:refresh",
                "admin:system:user-group:view-detail",
                "admin:system:user-group:view-members"
            ],
            "resource_key": "admin:system:user-group"
        },
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:user-group:create"
            ],
            "resource_key": "admin:system:user-group"
        },
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:user-group:edit",
                "admin:system:user-group:add-members",
                "admin:system:user-group:remove-members"
            ],
            "resource_key": "admin:system:user-group"
        },
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:user-group:delete",
                "admin:system:user-group:delete-selected"
            ],
            "resource_key": "admin:system:user-group"
        }
    ],
    "endpoints": [
        {
            "path": "/role",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/role/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/role/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/role",
            "method": "POST",
            "protocol": "http"
        },
        {
            "path": "/role/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/role/{id}",
            "method": "DELETE",
            "protocol": "http"
        },
        {
            "path": "/action",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/catalog",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/group",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/dictionary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/dictionary/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/action",
            "method": "POST",
            "protocol": "http"
        },
        {
            "path": "/action/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/action/{id}",
            "method": "DELETE",
            "protocol": "http"
        },
        {
            "path": "/user",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user",
            "method": "POST",
            "protocol": "http"
        },
        {
            "path": "/user/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/user/{id}",
            "method": "DELETE",
            "protocol": "http"
        },
        {
            "path": "/user-group",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user-group/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user-group/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user-group/{id}/member",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user-group",
            "method": "POST",
            "protocol": "http"
        },
        {
            "path": "/user-group/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/user-group/{id}",
            "method": "DELETE",
            "protocol": "http"
        }
    ],
    "feature_keys": null
}'::JSONB),
    (37, 'admin:system:developer', '76JJGCBN', 'admin:update:system:developer', '{
    "ui": [
        {
            "menu_key": "admin:menu:system:role",
            "menu_path": "/system/role",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:role:query",
                "admin:btn:system:role:reset-query",
                "admin:btn:system:role:refresh",
                "admin:btn:system:role:view-detail",
                "admin:btn:system:role:create",
                "admin:btn:system:role:edit",
                "admin:btn:system:role:delete",
                "admin:btn:system:role:delete-selected"
            ],
            "parent_menu_key": "admin:menu:system"
        },
        {
            "menu_key": "admin:menu:system:action",
            "menu_path": "/system/action",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:action:query",
                "admin:btn:system:action:reset-query",
                "admin:btn:system:action:refresh",
                "admin:btn:system:action:view-detail",
                "admin:btn:system:action:create",
                "admin:btn:system:action:create-group",
                "admin:btn:system:action:edit",
                "admin:btn:system:action:reorder-groups",
                "admin:btn:system:action:delete",
                "admin:btn:system:action:delete-selected",
                "admin:btn:system:action:delete-group"
            ],
            "parent_menu_key": "admin:menu:system"
        },
        {
            "menu_key": "admin:menu:system:user",
            "menu_path": "/system/user",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:user:query",
                "admin:btn:system:user:reset-query",
                "admin:btn:system:user:refresh",
                "admin:btn:system:user:view-detail",
                "admin:btn:system:user:create",
                "admin:btn:system:user:edit",
                "admin:btn:system:user:approve",
                "admin:btn:system:user:lock",
                "admin:btn:system:user:unlock",
                "admin:btn:system:user:unlock-password",
                "admin:btn:system:user:delete",
                "admin:btn:system:user:delete-selected"
            ],
            "parent_menu_key": "admin:menu:system"
        },
        {
            "menu_key": "admin:menu:system:user-group",
            "menu_path": "/system/user-group",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:user-group:query",
                "admin:btn:system:user-group:reset-query",
                "admin:btn:system:user-group:refresh",
                "admin:btn:system:user-group:view-detail",
                "admin:btn:system:user-group:view-members",
                "admin:btn:system:user-group:create",
                "admin:btn:system:user-group:edit",
                "admin:btn:system:user-group:add-members",
                "admin:btn:system:user-group:remove-members",
                "admin:btn:system:user-group:delete",
                "admin:btn:system:user-group:delete-selected"
            ],
            "parent_menu_key": "admin:menu:system"
        },
        {
            "menu_key": "admin:menu:system:developer",
            "menu_path": "/system/developer",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:developer:compare-online",
                "admin:btn:system:developer:refresh",
                "admin:btn:system:developer:publish"
            ],
            "parent_menu_key": "admin:menu:system"
        }
    ],
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:role:query",
                "admin:system:role:reset-query",
                "admin:system:role:refresh",
                "admin:system:role:view-detail"
            ],
            "resource_key": "admin:system:role"
        },
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:role:create"
            ],
            "resource_key": "admin:system:role"
        },
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:role:edit"
            ],
            "resource_key": "admin:system:role"
        },
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:role:delete",
                "admin:system:role:delete-selected"
            ],
            "resource_key": "admin:system:role"
        },
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:action:query",
                "admin:system:action:reset-query",
                "admin:system:action:refresh",
                "admin:system:action:view-detail"
            ],
            "resource_key": "admin:system:action"
        },
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:action:create",
                "admin:system:action:create-group"
            ],
            "resource_key": "admin:system:action"
        },
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:action:edit",
                "admin:system:action:reorder-groups"
            ],
            "resource_key": "admin:system:action"
        },
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:action:delete",
                "admin:system:action:delete-selected",
                "admin:system:action:delete-group"
            ],
            "resource_key": "admin:system:action"
        },
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:user:query",
                "admin:system:user:reset-query",
                "admin:system:user:refresh",
                "admin:system:user:view-detail"
            ],
            "resource_key": "admin:system:user"
        },
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:user:create"
            ],
            "resource_key": "admin:system:user"
        },
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:user:edit",
                "admin:system:user:approve",
                "admin:system:user:lock",
                "admin:system:user:unlock",
                "admin:system:user:unlock-password"
            ],
            "resource_key": "admin:system:user"
        },
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:user:delete",
                "admin:system:user:delete-selected"
            ],
            "resource_key": "admin:system:user"
        },
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:user-group:query",
                "admin:system:user-group:reset-query",
                "admin:system:user-group:refresh",
                "admin:system:user-group:view-detail",
                "admin:system:user-group:view-members"
            ],
            "resource_key": "admin:system:user-group"
        },
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:user-group:create"
            ],
            "resource_key": "admin:system:user-group"
        },
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:user-group:edit",
                "admin:system:user-group:add-members",
                "admin:system:user-group:remove-members"
            ],
            "resource_key": "admin:system:user-group"
        },
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:user-group:delete",
                "admin:system:user-group:delete-selected"
            ],
            "resource_key": "admin:system:user-group"
        },
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:developer:compare-online",
                "admin:system:developer:refresh"
            ],
            "resource_key": "admin:system:developer"
        },
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:developer:publish"
            ],
            "resource_key": "admin:system:developer"
        }
    ],
    "endpoints": [
        {
            "path": "/role",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/role/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/role/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/role",
            "method": "POST",
            "protocol": "http"
        },
        {
            "path": "/role/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/role/{id}",
            "method": "DELETE",
            "protocol": "http"
        },
        {
            "path": "/action",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/catalog",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/group",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/action/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/dictionary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/dictionary/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/action",
            "method": "POST",
            "protocol": "http"
        },
        {
            "path": "/action/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/action/{id}",
            "method": "DELETE",
            "protocol": "http"
        },
        {
            "path": "/user",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user",
            "method": "POST",
            "protocol": "http"
        },
        {
            "path": "/user/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/user/{id}",
            "method": "DELETE",
            "protocol": "http"
        },
        {
            "path": "/user-group",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user-group/summary",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user-group/{id}",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user-group/{id}/member",
            "method": "GET",
            "protocol": "http"
        },
        {
            "path": "/user-group",
            "method": "POST",
            "protocol": "http"
        },
        {
            "path": "/user-group/{id}",
            "method": "PATCH",
            "protocol": "http"
        },
        {
            "path": "/user-group/{id}",
            "method": "DELETE",
            "protocol": "http"
        }
    ],
    "feature_keys": null
}'::JSONB);

SELECT setval(pg_get_serial_sequence('t_action', 'id'), (SELECT MAX(id) FROM t_action), TRUE);

-- +migrate Down
TRUNCATE TABLE t_action RESTART IDENTITY;
