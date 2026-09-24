-- +migrate Up
INSERT INTO t_action (id, name, action_group, code, word, resource, menu, btn) VALUES
    (1, 'All Permissions', 'General', 'SN2837AY', '*', '*', '*', '*'),
    (2, 'Default Permissions', 'General', 'KHXK5JVL', 'default', 'POST|/auth/logout|/auth.v1.Auth/Logout
GET|/auth/info|/auth.v1.Auth/Info
POST|/auth/challenge|/auth.v1.Auth/Challenge
POST|/auth/captcha|/auth.v1.Auth/Captcha
PATCH|/auth/change/pwd|/auth.v1.Auth/Pwd
POST|/auth/captcha/verify
PATCH|/auth/reset/pwd', '/dashboard/overview
/user/index', ''),
    (3, 'Dashboard', 'General', '2QKHTYEE', 'dashboard', '', '/dashboard/overview', ''),
    (4, 'User Read', 'User', 'GRNA3NPV', 'user.read', 'GET|/user|/auth.v1.User/ListUsers
GET|/user/*|/auth.v1.User/GetUser', '/system/user', 'system.user.read'),
    (5, 'User Create', 'User', '2LV9MDWB', 'user.create', 'POST|/user|/auth.v1.User/CreateUser', '/system/user', 'system.user.create'),
    (6, 'User Update', 'User', 'NME3CT5H', 'user.update', 'PATCH|/user/*|/auth.v1.User/UpdateUser', '/system/user', 'system.user.update'),
    (7, 'User Delete', 'User', 'EQH37R9C', 'user.delete', 'DELETE|/user/*|/auth.v1.User/DeleteUser', '/system/user', 'system.user.delete'),
    (8, 'User Group Read', 'User Group', 'V2HRXGW9', 'user.group.read', 'GET|/user-group|/auth.v1.UserGroup/ListUserGroups
GET|/user-group/*|/auth.v1.UserGroup/GetUserGroup', '/system/group', 'system.user.group.read'),
    (9, 'User Group Create', 'User Group', 'GGKPXAL6', 'user.group.create', 'POST|/user-group|/auth.v1.UserGroup/CreateUserGroup', '/system/group', 'system.user.group.create'),
    (10, 'User Group Update', 'User Group', 'JM3TT968', 'user.group.update', 'PATCH|/user-group/*|/auth.v1.UserGroup/UpdateUserGroup', '/system/group', 'system.user.group.update'),
    (11, 'User Group Delete', 'User Group', 'JE45TMPQ', 'user.group.delete', 'DELETE|/user-group/*|/auth.v1.UserGroup/DeleteUserGroup', '/system/group', 'system.user.group.delete'),
    (12, 'Role Read', 'Role', 'AS2V9HND', 'role.read', 'GET|/role|/auth.v1.Role/ListRoles
GET|/role/*|/auth.v1.Role/GetRole', '/system/role', 'system.role.read'),
    (13, 'Role Create', 'Role', '88BA22VF', 'role.create', 'POST|/role|/auth.v1.Role/CreateRole', '/system/role', 'system.role.create'),
    (14, 'Role Update', 'Role', 'GE5YBVDN', 'role.update', 'PATCH|/role/*|/auth.v1.Role/UpdateRole', '/system/role', 'system.role.update'),
    (15, 'Role Delete', 'Role', 'AY6QE7QG', 'role.delete', 'DELETE|/role/*|/auth.v1.Role/DeleteRole', '/system/role', 'system.role.delete'),
    (16, 'Action Read', 'Action', '42TMWNP3', 'action.read', 'GET|/action|/auth.v1.Action/ListActions
GET|/action/*|/auth.v1.Action/GetAction', '/system/action', 'system.action.read'),
    (17, 'Action Create', 'Action', 'SXPYFM3K', 'action.create', 'POST|/action|/auth.v1.Action/CreateAction', '/system/action', 'system.action.create'),
    (18, 'Action Update', 'Action', '8VCXMSCW', 'action.update', 'PATCH|/action/*|/auth.v1.Action/UpdateAction', '/system/action', 'system.action.update'),
    (19, 'Action Delete', 'Action', '86QSDSRL', 'action.delete', 'DELETE|/action/*|/auth.v1.Action/DeleteAction', '/system/action', 'system.action.delete'),
    (20, 'Whitelist Read', 'Whitelist', 'ALX2LHB2', 'whitelist.read', 'GET|/whitelist|/auth.v1.Whitelist/ListWhitelists
GET|/whitelist/*|/auth.v1.Whitelist/GetWhitelist', '/system/whitelist', 'system.whitelist.read'),
    (21, 'Whitelist Create', 'Whitelist', 'ALCARRQ8', 'whitelist.create', 'POST|/whitelist|/auth.v1.Whitelist/CreateWhitelist', '/system/whitelist', 'system.whitelist.create'),
    (22, 'Whitelist Update', 'Whitelist', '28FN73B3', 'whitelist.update', 'PATCH|/whitelist/*|/auth.v1.Whitelist/UpdateWhitelist', '/system/whitelist', 'system.whitelist.update'),
    (23, 'Whitelist Delete', 'Whitelist', 'E8SN4T9K', 'whitelist.delete', 'DELETE|/whitelist/*|/auth.v1.Whitelist/DeleteWhitelist', '/system/whitelist', 'system.whitelist.delete'),
    (24, 'Dictionary Read', 'Dictionary', 'D1CTREAD', 'dictionary.read', 'GET|/dictionary|/auth.v1.Dictionary/ListDictionaries
GET|/dictionary/*|/auth.v1.Dictionary/GetDictionary', '/system/dictionary', 'system.dictionary.read'),
    (25, 'Dictionary Create', 'Dictionary', 'D1CTCRTE', 'dictionary.create', 'POST|/dictionary|/auth.v1.Dictionary/CreateDictionary', '/system/dictionary', 'system.dictionary.create'),
    (26, 'Dictionary Update', 'Dictionary', 'D1CTUPDT', 'dictionary.update', 'PATCH|/dictionary/*|/auth.v1.Dictionary/UpdateDictionary', '/system/dictionary', 'system.dictionary.update'),
    (27, 'Dictionary Delete', 'Dictionary', 'D1CTDELE', 'dictionary.delete', 'DELETE|/dictionary/*|/auth.v1.Dictionary/DeleteDictionary', '/system/dictionary', 'system.dictionary.delete');

INSERT INTO t_role (id, name, word, action) VALUES
    (1, 'Admin', 'admin', 'SN2837AY'),
    (2, 'Guest', 'guest', '2QKHTYEE');

INSERT INTO t_user_group (id, name, word, action) VALUES
    (1, 'Read Only', 'readonly', 'GRNA3NPV,V2HRXGW9,AS2V9HND,42TMWNP3'),
    (2, 'Read Write', 'write', 'GRNA3NPV,2LV9MDWB,NME3CT5H,EQH37R9C,V2HRXGW9,GGKPXAL6,JM3TT968,JE45TMPQ,AS2V9HND,88BA22VF,GE5YBVDN,AY6QE7QG,42TMWNP3,SXPYFM3K,8VCXMSCW,86QSDSRL'),
    (3, 'No Delete', 'nodelete', 'GRNA3NPV,2LV9MDWB,NME3CT5H,V2HRXGW9,GGKPXAL6,JM3TT968,AS2V9HND,88BA22VF,GE5YBVDN,42TMWNP3,SXPYFM3K,8VCXMSCW');

-- Local seed credentials intentionally use the username as the password.
INSERT INTO t_user (id, role_id, username, code, password, status) VALUES
    (1, 1, 'super', '89HEK28Y', '$2a$10$Wx41MADqB3kuLD/bdD8DIeph55Oeo9HABKGt2p2ilqdkHAfZ7EGPm', 1),
    (2, 2, 'guest', '4VPNKE6M', '$2a$10$DzWbkb12WHoyj5UPIAdHBOYXFi6jn5gzQRUoOU8c6oZ9XJx9w6Tju', 1),
    (3, NULL, 'readonly', 'EXP78RGH', '$2a$10$e5bhTbZUo8JLBWOfxWY7A.BpiGCTgvV3PxIhPozM9UolwFuENjIZi', 1),
    (4, NULL, 'write', '6SHWH93V', '$2a$10$SB4cqKKE5t9ACvMXtHaJuOafN1nvJgdK.5XxVAITBCSsU.gSNTbAm', 1),
    (5, NULL, 'nodelete', 'JJHWJ9YJ', '$2a$10$.QZNNNrOuEdPvV0UdYwEae942Bb7Hr28Zb0vrbBRocHUkSpqk/Uqu', 1);

INSERT INTO t_user_user_group_relation (user_id, user_group_id) VALUES
    (3, 1),
    (4, 2),
    (5, 3);

INSERT INTO t_whitelist (id, category, resource) VALUES
    (1, 0, '/grpc.health.v1.Health/Check
/grpc.health.v1.Health/Watch'),
    (2, 1, '/grpc.health.v1.Health/Check
/grpc.health.v1.Health/Watch');

INSERT INTO t_dictionary (id, dictionary_key, name, value, description) VALUES
    (1, 'POINT_CAPTCHA_ENGLISH_CHARACTERS', 'Point Captcha English Characters', '["A","B","C","D","E","F","G","H","I","J","K","L","M","N","O","P","Q","R","S","T","U","V","W","X","Y","Z","0","1","2","3","4","5","6","7","8","9"]'::jsonb, 'Uppercase English letters and digits available to point captcha challenges.'),
    (2, 'POINT_CAPTCHA_CHINESE_CHARACTERS', 'Point Captcha Chinese Characters', '["啊","挨","皑","艾","鞍","俺","胺","盎","翱","奥","捌","笆","拔","把","罢","百","拜","班","颁","拌","办","帮","绑","镑","胞","剥","堡","报","爆","悲","背","狈","被","本","甭","逼","笔","蓖","毖","闭","辟","避","编","变","辫","彪","憋","斌","摈","柄","炳","菠","波","铂","帛","渤","卜","不","簿","猜","财","彩","餐","惭","舱","操","曹","侧","蹭","茶","搽","诧","搀","谗","产","猖","长","敞","倡","朝","吵","撤","澈","尘","陈","称","成","惩","逞","痴","池","耻","赤","充","崇","畴","筹","丑","出","锄","楚","搐","川","传","疮","闯","捶","春","淳","绰","雌","瓷","赐","囱","凑","簇","窜","脆","翠","寸","措","达","大","傣","代","逮","丹","掸","但","弹","党","刀","岛","稻","德","蹬","瞪","低","笛","翟","地","弟","掂","点","电","惦","碉","凋","钓","碟","叠","钉","锭","东","动","冻","抖","逗","毒","堵","杜","渡","锻","堆","对","敦","盾","多","躲","剁","峨","讹","厄","饿","耳","二","罚","阀","帆","矾","凡","范","泛","肪","妨","放","飞","吠","沸","吩","坟","奋","愤","枫","风","冯","奉","夫","扶","氟","服","福","抚","斧","腐","覆","付","负","妇","噶","概","干","竿","感","冈","肛","杠","高","搞","哥","鸽","割","蛤","个","跟","庚","梗","恭","公","巩","共","苟","购","菇","沽","古","股","固","剐","乖","关","观","惯","广","圭","龟","诡","跪","滚","郭","过","海","骇","韩","寒","翰","憾","汉","航","毫","号","荷","禾","盒","涸","贺","痕","哼","恒","虹","宏","侯","候","忽","葫","糊","唬","户","华","划","徊","欢","还","唤","涣","慌","蝗","惶","恍","辉","蛔","慧","贿","汇","绘","魂","活","或","货","基","积","迹","姬","吉","籍","疾","级","脊","冀","剂","寂","既","继","夹","荚","甲","价","歼","笺","兼","缄","碱","简","减","践","箭","剑","溅","姜","疆","讲","降","焦","浇","搅","侥","饺","教","叫","接","阶","桔","竭","解","芥","疥","筋","今","锦","靳","烬","荆","睛","惊","井","静","镜","竟","窘","玖","灸","救","咎","拘","居","咀","聚","具","锯","炬","娟","绢","抉","觉","均","君","竣","喀","开","慨","勘","康","抗","拷","坷","磕","咳","刻","肯","坑","孔","扣","哭","库","挎","筷","宽","狂","旷","岿","魁","愧","捆","廓","喇","辣","赖","拦","澜","览","滥","廊","捞","老","烙","雷","累","擂","棱","梨","狸","理","礼","栗","砾","傈","立","力","联","镰","帘","恋","凉","良","晾","聊","寥","了","料","劣","磷","邻","赁","菱","伶","灵","另","榴","刘","柳","咙","垄","娄","陋","颅","卤","碌","鹿","录","吕","履","氯","滤","孪","掠","伦","纶","罗","骡","骆","麻","马","埋","迈","馒","曼","芒","忙","茅","铆","帽","玫","酶","眉","美","媚","们","盟","孟","糜","弥","泌","棉","免","缅","瞄","庙","民","敏","螟","命","蘑","摩","末","沫","谋","牡","母","募","睦","哪","那","乃","南","挠","闹","内","霓","拟","腻","拈","捻","鸟","聂","镍","狞","泞","钮","农","怒","疟","糯","鸥","偶","爬","琶","徘","潘","畔","乓","胖","炮","呸","裴","佩","砰","彭","硼","鹏","砒","劈","脾","匹","譬","骗","票","拼","聘","萍","评","泼","魄","扑","葡","埔","浦","期","妻","漆","棋","崎","祈","起","启","气","泣","恰","钎","签","黔","前","谴","歉","羌","强","敲","乔","撬","俏","且","侵","勤","禽","轻","清","情","庆","丘","囚","区","屈","取","去","醛","拳","劝","却","雀","燃","瓤","让","惹","人","任","纫","戎","融","容","柔","儒","乳","褥","瑞","若","萨","塞","伞","丧","嫂","涩","砂","纱","筛","苫","煽","擅","汕","墒","晌","梢","芍","少","奢","舍","慑","设","伸","绅","婶","慎","甥","省","圣","施","尸","拾","蚀","矢","驶","士","拭","是","适","饰","室","手","寿","瘦","梳","叔","疏","熟","署","属","树","墅","恕","衰","栓","爽","税","舜","朔","嘶","丝","嗣","似","耸","宋","艘","苏","速","溯","酸","虽","髓","遂","损","梭","索","他","獭","胎","台","态","贪","檀","谭","袒","炭","堂","唐","淌","涛","桃","陶","藤","梯","提","啼","惕","天","甜","腆","眺","帖","汀","亭","通","同","桶","统","头","突","涂","吐","推","褪","臀","脱","驼","唾","洼","袜","弯","丸","挽","惋","腕","枉","望","巍","韦","唯","维","伟","纬","畏","位","慰","温","纹","问","挝","窝","握","钨","屋","梧","武","舞","戊","物","误","西","嘻","牺","悉","熄","犀","席","铣","戏","匣","暇","下","掀","鲜","贤","涎","险","腺","宪","相","箱","翔","想","巷","象","削","消","晓","肖","楔","鞋","携","谐","蟹","谢","芯","新","衅","惺","型","醒","姓","匈","熊","朽","袖","需","须","酗","畜","绪","喧","玄","绚","穴","勋","询","殉","逊","押","丫","崖","哑","焉","淹","蜒","颜","沿","衍","燕","唁","宴","央","扬","洋","仰","漾","瑶","窑","舀","耀","爷","页","曳","液","揖","衣","移","疑","彝","已","艺","邑","臆","亦","忆","溢","译","绎","殷","姻","寅","隐","婴","缨","荧","盈","映","佣","雍","泳","勇","优","由","油","友","釉","迂","榆","余","鱼","隅","与","语","域","遇","愈","誉","预","鸳","垣","辕","猿","远","院","跃","月","耘","陨","酝","匝","栽","载","攒","脏","糟","早","噪","燥","则","增","扎","轧","眨","乍","斋","寨","詹","斩","蘸","站","樟","张","丈","胀","招","赵","肇","哲","者","浙","甄","针","疹","镇","睁","怔","政","郑","支","肢","织","植","侄","趾","志","至","峙","稚","滞","中","衷","重","周","诌","帚","昼","蛛","诸","烛","嘱","助","铸","祝","拽","撰","桩","撞","锥","缀","捉","琢","着","咨","滋","仔","自","棕","综","走","足","诅","钻","最","昨","做"]'::jsonb, 'One thousand common simplified Chinese characters sampled from the GB2312 level-one repertoire.');

SELECT setval(pg_get_serial_sequence('t_action', 'id'), (SELECT MAX(id) FROM t_action), true);
SELECT setval(pg_get_serial_sequence('t_role', 'id'), (SELECT MAX(id) FROM t_role), true);
SELECT setval(pg_get_serial_sequence('t_user_group', 'id'), (SELECT MAX(id) FROM t_user_group), true);
SELECT setval(pg_get_serial_sequence('t_user', 'id'), (SELECT MAX(id) FROM t_user), true);
SELECT setval(pg_get_serial_sequence('t_whitelist', 'id'), (SELECT MAX(id) FROM t_whitelist), true);

SELECT setval(pg_get_serial_sequence('t_dictionary', 'id'), (SELECT MAX(id) FROM t_dictionary), true);

-- +migrate Down
DELETE FROM t_dictionary WHERE id IN (1, 2);

DELETE FROM t_user_user_group_relation
WHERE (user_id, user_group_id) IN (
    (3, 1),
    (4, 2),
    (5, 3)
);

DELETE FROM t_user WHERE id IN (
    1,
    2,
    3,
    4,
    5
);

DELETE FROM t_user_group WHERE id IN (
    1,
    2,
    3
);

DELETE FROM t_role WHERE id IN (1, 2);

DELETE FROM t_action WHERE id IN (
    1,
    2,
    3,
    4,
    5,
    6,
    7,
    8,
    9,
    10,
    11,
    12,
    13,
    14,
    15,
    16,
    17,
    18,
    19,
    20,
    21,
    22,
    23,
    24,
    25,
    26,
    27
);

DELETE FROM t_whitelist WHERE id IN (1, 2);
