# Devin 版本适配

Hub 把后台模型挂进 Devin 的做法，是在启动拦截时，对安装目录里的压缩脚本做整段原文替换。压缩后的函数名每次发版都会变。对不上的那一段会跳过，日志里出现「对不上，没有改动」，对应的列表、图标、倍率或叫醒就失效。转发本身在 Go 里，不随 Devin 发版失效。

当前已对齐的版本：

| 产品版本 | 内部 VS Code | 提交 | 安装包日期 |
|---|---|---|---|
| 更早的一版 | | | 第一组锚点，无版本后缀 |
| Devin 1.126 | 1.126.0 | `92c83011` | |
| Devin Desktop 3.11.238 | 1.126.0 | `9e593c73d4ca24802eb3b302035f33cedad59802` | 2026-10-30 |
| Devin Desktop 3.12.28 | 1.141.2 | `91c3aec41579d2c1795b64f82fa6a8a202ea4c69` | 2026-11-07 |

关于里的 VS Code OSS 版本可以不动，Devin 自己的版本号和提交已经换了。以 `resources\app\product.json` 里的 `windsurfVersion` 和关于里的 Devin Desktop Commit 为准。

## 每次改动必须成对

一组锚点是两段字符串：安装文件里的原文，和替换后的文字。启动时按「3.12.28 → 238 → 1.126 → 更早」的顺序试，命中哪组用哪组。旧组留着，不要删。

规则：

- 原文在目标文件里必须正好出现 1 次。0 次就是这版改名了；多于 1 次就把片段再加长。
- 替换后的文字也要能唯一对上，还原时按同一组换回去。3.11.238 的列表用 `wr`，还原必须回到 `wr`，不能回到 1.126 的 `tr`。
- 已经装过上一版补丁时，再加一条「旧替换结果 → 新替换结果」，让已改过的安装也能升上来。模式按钮那次就是这样：`agentShownKeepBroad` 升到带 `category==="model"` 的 `agentShownKeep`。
- 只改符号，不改行为。Hub 模型编号始终是 `编号@思考档@上下文`，三段，空思考档写成 `-`。没有 `@` 的点选不通知 Hub。Agent 发出去的名字仍换成 `swe-1-6-slow`，真正的编号由 Hub 按会话记住。
- 片段里的单字母都是压缩后的局部变量，换版本会变。`t.category`、`configId:J`、`bailanSyncShown(c,H)` 里的 `t`、`J`、`c`、`H` 都要从新文件里抄，不能沿用 3.11.238 的字母。`useMemo` / `useCallback` / `jsx` 前面的导入名（`tK`、`Z`、`tY`、`q`、`J`、`G`、`ht`、`vo`、`Yo`）同样算锚点的一部分。

源码在 `local-go/internal/devin/`：

| 文件 | 管什么 |
|---|---|
| `clientpatch.go` | 普通对话列表、点选通知、排序菜单、倍率、叫醒 |
| `patchdata/picker_before_*.txt`、`picker_after_*.txt` | 普通对话列表的整段原文 |
| `agentpatch.go` | Agent 列表、排序、点选、新建会话、思考档匹配、上下文用量、没选模型不能发送 |
| `vendoricon.go` | 两边的图标，以及普通对话里「当前显示的模型」 |
| `devin238_install_test.go` | 拿 3.11.238 的安装目录做一轮套上再还原。文件不在时跳过 |
| `devin312_install_test.go` | 拿 3.12.28 的 `app-initial-DlcIkwcB.js`、`app-initial-B6DX4ee6.js`、`extension.js` 做套上再还原 |
| `requiremodel_test.go` | 没选模型不能发送。按句子找文件，不写死哈希 |

`ApplyClientPatches` 按这个顺序写，每一项单独成败，互不挡住：

1. Agent 模型列表
2. Agent 排序菜单
3. Agent 上下文用量
4. Agent 图标
5. 没选模型不能发送
6. 普通对话列表（含点选、图标、当前模型）
7. 倍率角标
8. 叫醒

## 安装目录里动哪些文件

Devin 装在 `E:\Devin` 时，资源根是 `E:\Devin\resources`。文件名里的哈希每次发版都会变，按内容搜，不要按文件名搜。

| 路径 | 3.11.238 上的文件 | 内容 |
|---|---|---|
| `app\out\vs\workbench\windsurf-chat-client\index.js` | 仍是这一个文件 | 普通对话列表、点选、图标、当前模型、IDE 倍率 |
| `app\out\vs\workbench\devin-webapp-host\assets\app-initial-*.js` | `app-initial-DnnoLajw.js` | Agent 列表、点选、当前模型、模式 |
| 同上 | `app-initial-CECksEiP.js` | Agent 排序、图标、下拉行、Agent 倍率 |
| 同上 | `app-initial-n5TcvL6i.js` | 没有模式选项时会造一个英文 Mode |
| `app\extensions\windsurf\dist\extension.js` | 仍是这一个文件 | 叫醒 |

写文件前 Devin 必须关掉。拦截启动会自己关。还原时会清 `%APPDATA%\Devin\Code Cache`。补丁里的通知地址写死 `http://127.0.0.1:5099`，Hub 要听这个端口。监听被占、改到别的端口时，列表能显示，点选和叫醒到不了 Hub。

## 先分清是改名，还是转发

脚本对上了，模型仍可能不出现、发错或按钮显示 Mode。这几处在 Go 里，换 Devin 版本通常不用动。

模型进列表靠目录响应，不靠补丁造数据。`local-go/internal/devincatalog/catalog.go` 改写 `GetUserStatus`、`GetCascadeModelConfigs`、`GetCliModelConfigs`、`GetCommandModelConfigs`，给每条加上 `devinhub-family-` 前缀，显示档位是 `8`（和官方模型同一张表）。`GetUserJwt`、`GetCliTeamSettings` 必须原样回传，改写会表现为登录失效。补丁再把选择器收成只留这个前缀，组名固定「未分类」，和 `uncategorizedName` 一致。补丁日志已经成功、列表却是空的，先查目录注入和显示档位，再查锚点。

转发看 `proxy_日期.log`，客户版在 `%LOCALAPPDATA%\BaiLan Devin Relay Client\logs\`，完整版在 `%LOCALAPPDATA%\BaiLan Proxy Manager\logs\`。界面会滤掉「已把你的模型写进」「已接上模型变化」「已在模型名字旁边」这几句，对错以这个文件为准。

| 日志 | 含义 |
|---|---|
| `原文：swe-1-6-slow` | 补丁已把带 `@` 的编号换成 Devin 允许的名字，Hub 仍按记住的编号转发 |
| `按会话` | 请求对上了记住的模型。字段对上，或正文里只扫到一个模型 |
| `[转发对照]` | 字段里的编号对不上。客户版会丢掉带「请求里的会话编号」的句子，所以改成这句。后面若正文扫到唯一的会话名，仍按会话转发 |
| `Devin 原文未识别` | 请求里的名字既不是三段 `@` 编号，也不是目录里的模型 |

记住会话时同时存三份：完整编号、把 `/` 换成 `_` 的形式、最后一个 `/` 后面的短名。3.12.28 桌面日志里的会话是 `devin-acp_devin-cli_名字`，脚本里存的是 `devin-acp/devin-cli/名字`。只存斜杠形式时，后一句会对不上。

后续消息不再经过模型按钮。`GetChatMessage` 的正文是 gzip 的 Connect 帧，会话名在解压之后。`MessagePayload` 先去掉帧头再解压，直接在原始字节里搜名字一定是 0。字段对不上才扫正文，打字和拉目录不扫。短名如果是某个更长名字的后缀，丢掉短名。剩下的命中如果模型、思考档或上下文不一致，不猜，退回全局模型。编号字段已经对上时不再扫正文。

`SelectedRelayChoice` 见到 `@` 就要求正好三段。写成 `编号@思考档` 两段会被当成无法识别。单档模型也要带上思考档和上下文。

模式按钮在取值函数已经带上 `category==="model"` 之后仍显示 Mode，说明这个会话里没有模式选项。`app-initial-n5TcvL6i.js` 会补一个 `{id:mode, name:Mode}`。那是 Devin 自己的退回，再改模型取值函数没用。

## 六处分别怎么找

下面的「稳定文字」是压缩后仍然认得的句子。先在干净安装里搜到它，再向两边扩，直到整段只出现 1 次。PowerShell 会吃掉反引号，提取用 Python，见文末。

### 1. 普通对话列表

文件：`windsurf-chat-client\index.js`。

稳定文字：`Available models`。它旁边是两个 `useMemo`，一个摊平选项，一个按分组收拢。

3.11.238：摊平变量 `j`，分组变量 `q`，认选项 `rkL`，映射 `n4m`，列表 `N` / `L`，开关 `U`。原文以 `]:[]` 结束，末尾没有换行。替换后的 `picker_after_238.txt` 只保留 `devinhub-family-`，没有分组时归进「未分类」，并且停在 `return sorts` 加一个换行，后面原来的 `},[依赖])` 留在文件里。

点选通知紧挨 `pendingSwitchTarget` 和 `value:e.modelUid`。3.11.238 是 `et`、`tld`、`el0`，配置编号 `J`。1.126 是 `Z`、`tlc`、`elZ`，配置编号 `U`。通知内容是现成的 `chatSelectNotify`，往回调开头插，不要改端口 `127.0.0.1:5099`。

排序菜单稳定文字是 `modelSelect.orderBy`，旁边把 `filter(e=>e!==n)` 换成 `slice()`，默认排序的名字才点得到。3.11.238 这段仍是 `f=(0,tK.useMemo)`，和 1.126 相同，没有新锚点。

### 2. 普通对话当前模型和同步

仍在 `index.js`。稳定文字：`activeSessionId:` 和 `toggleModelSelector.on`。

3.11.238：取值函数 `rEh`，结果变量 `H`，配置编号 `J`，同步用的 ref 是 `Y`。套上 `bailanChatModel(c, …)`，再加 `(0,tK.useEffect)(()=>{bailanSyncShown(c,H)},[c,H])`。

1.126：取值函数 `rEu`，结果变量 `J`，ref 是 `V`，同步 `bailanSyncShown(c,J)`。

不要再写渲染时的 `(0,tK.useEffect)(()=>{bailanSyncModel(J)},[J])`。切到 IDE 用 `bailanSyncShown`。`patchChatSession` 里还留着把那句删掉的逻辑。

插入 `bailanChatModel` 时，锚点是厂商图标函数的开头，3.11.238 是 `function rkv(e,n=16){if(void 0===e)return null;`。这个开头对不上时，整段会话模型都会报「对不上」，连列表一起不写。所以新版本先补图标函数名。

### 3. Agent 列表和点选

目录：`devin-webapp-host\assets\`，扫全部 `app-initial-*.js`。

稳定文字：`models.flatMap`、`catalogMisses`、`LocalModelPicker:Select:Model`、`session/set_config_option`。

3.11.238 对照：

| 行为 | 1.126 | 3.11.238 |
|---|---|---|
| 摊平，认识的 / 禁用的 | `tr` / `nr` | `wr` / `Tr` |
| 认带 `@` 的编号 | `Ca` / `Sa` | `no` / `to` |
| 新建会话赋值 | `Ca(u,r,d,a)` | `no(u,r,d,a)` |
| 记住函数的插入点 | `function La(e,t){` | `function to(e,t){return yr(e.options).some(e=>e.value===t)}` |
| 清掉不认识的值 | `Ra.useEffect` | `vo.useEffect` |
| 思考档匹配 | `te` / `fa` | `ne` / `Ga` |
| 发送前回话 | `go.useCallback` | `Yo.useCallback` |

替换后的摊平只留 `family.uid` 以 `devinhub-family-` 开头的行，再插进组名「未分类」。点选那段 `ee=(0,Z.useCallback)` 在 3.11.238 没变。回放 `method:\`session/set_config_option\`` 也没变。排序默认值 `isDefault:e.isDefault` 仍是同一句，强制写成 `isDefault:!1`。

列表、点选、新建会话、模式取值、思考档匹配写在同一个 `app-initial-*.js` 里。摊平原文对上就会写盘，并打出「已把你的模型写进 Agent 模式的列表」。后面的句子出现次数不是 1 时会被悄悄跳过，这句成功不代表新建会话和模式保护也写上了。写完在该文件里再搜 `set_session_model` 和 `t.category==="model"`（字母按新版本抄）。

当前显示的模型是 `{value:d??n?.value??l,onSelect:`。这个取值函数模型和模式共用。开头必须是：

```text
if(!(t&&t.category==="model"))return d??n?.value??l;
```

否则模式对不上可选项，按钮退回 `agents.mode`，界面上就是英文 Mode。模式选项本身由 `id===\`mode\`` 且 `type===\`select\`` 找出来，模型由 `category===\`model\`` 找出来。

### 4. Agent 排序

稳定文字：`modelSelect.orderBy`，以及 `h=(0,XX.useMemo)(()=>n.filter(e=>e!==t),[n,t])`。

3.11.238 把 `ut` 换成了 `ht`，替换成 `n.slice()`。

`patchAgentSortMenu` 只要扫到的某个文件里已经有替换后的那句，就当成已经写过，直接返回。所以新句子必须在整个 assets 目录里唯一。

### 5. 图标

稳定文字：`flex size-[18px] shrink-0 items-center justify-center`，以及 `e.icon??`。

普通对话在 `index.js`。3.11.238 厂商图标函数是 `rkv`，按模型找图标是 `rwO`。1.126 是 `rw0` / `rwo`。更早是 `rwX` / `rwr`。行是 `e.icon??函数(e.provider)`，卡片是 `R.icon??(n?按模型找(e.modelUid)??函数(R.provider):函数(R.provider))`。

Agent 有两处，都在 assets 里，3.11.238 都改叫 `Kn`（1.126 是 `Vn`）：

- 选项组件：`e.icon??Kn(e.provider)`。`function Kn` 在别的文件里也会出现，不能拿函数名单独当命中。
- 用户看见的下拉行：`n=C.icon??(t?Dt(e.modelUid)??Kn(C.provider):Kn(C.provider));return n?(0,J.jsx)(\`span\`...`。只改上面那个组件，列表里仍然没图标。
- 卡片还有一句 `n=S.icon??(t?St(e.modelUid)??Vn(S.provider):Vn(S.provider))`。3.11.238 这句没改名，对不上就跳过。下一版把 `C.icon??` 和 `S.icon??` 两句都搜出来。

Devin 自带厂商表只有 windsurf、openai、anthropic、google、xai、deepseek、moonshot、qwen、zai、minimax、nvidia、thinking_machines、meta。小米、智谱这些本来就会空，要靠 `bailanSrc` 按前缀补。

`bailanSrc` 开头是 `if(!e||e.provider)return""`。选项对象上带了 `provider` 就会直接空。补图时只传 `{modelUid, label}`，不要把整个选项传进去。前缀按编号和名字的开头匹配，`mimo` / `xiaomi` 用小米图，对不上的留空，不会落到小米图上。

`bailanSrc` 的函数体写进安装文件后，还原按整段原文换回。改函数体之前，要同时改还原用的旧文字，否则已安装的那份对不上。

### 6. 倍率

稳定文字：`e.isBeta?`、`fastStatus?.isActive`、`flex items-center gap-1`。

普通对话用 `tY.jsx`，变量是 `a` / `r` / `i` / `n`。3.11.238 的组件是 `eTK`、`rkm`、`rku`、`rkc`、`rkp`。1.126 是 `rw$`、`rwV`、`rwH`、`rwY`。原文收到 `},e.key))})` 为止，不要把后面的 `}})))()` 卷进来。

Agent 用 `G.jsx`，变量是 `i` / `n` / `r` / `t`，在 `app-initial-CECksEiP.js`。3.11.238 是 `Qe`、`En`、`wn`、`m`、`Dn`。

替换只是在数组开头多塞一项 `{key:"rate", label:倍率}`。

### 7. 叫醒

文件：`extensions\windsurf\dist\extension.js`。

稳定文字：`},1e4)}async function`。后面那个函数名就是锚点。

- 更早：`},1e4)}async function H`，刷新目录调用 `G(e,0)`。
- 1.126 和 3.11.238：`},1e4)}async function j`，刷新目录调用 `B(e,0)`。3.11.238 没有新锚点。

新版本如果这句对不上，先找到 `MetadataProvider.getInstance().getMetadata()` 附近、真正拿 `apiKey` 再去拉用户状态的那个调用，用它替换 `B(e,0)`。不要在新包里调用旧包的 `G`。

还原不靠锚点，而是删掉 `;function bailanArm(seen)` 到 `bailanArm(-1);` 这一段。

## 3.11.238 符号对照

| 功能 | 更早 | 1.126 | 3.11.238 |
|---|---|---|---|
| 对话列表 | `picker_before.txt` | `B` `F` `rkl` `n4l` | `j` `q` `rkL` `n4m` |
| 对话点选 | `Z` `tlc` `elZ` 配置 `U` | 同左 | `et` `tld` `el0` 配置 `J` |
| 对话当前模型 | `rEs` 变量 `J` | `rEu` 变量 `J` | `rEh` 变量 `H` |
| 切到 IDE | ref `V`，同步 `J` | 同左 | ref `Y`，同步 `H` |
| 对话图标 | `rwX` / `rwr` | `rw0` / `rwo` | `rkv` / `rwO` |
| 对话倍率 | `rwH` `rwJ` `rwV` `rwz` | `rw$` `rwV` `rwH` `rwY` | `eTK` `rkm` `rku` `rkc` `rkp` |
| Agent 列表 | `tr` / `nr` | 同左 | `wr` / `Tr` |
| Agent 排序 | `ut.useMemo` | 同左 | `ht.useMemo` |
| Agent 认编号 | `Ca` / `Sa` | 同左 | `no` / `to` |
| Agent 清值 | `Ra.useEffect` | 同左 | `vo.useEffect` |
| Agent 思考档 | `te` / `fa` | 同左 | `ne` / `Ga` |
| Agent 发送 | `go.useCallback` | 同左 | `Yo.useCallback` |
| Agent 图标 | `Vn` | 同左 | `Kn`，下拉行是 `n=C.icon??` |
| Agent 倍率 | `Ye` `xn` `yn` `Sn` | 同左 | `Qe` `En` `wn` `Dn` |
| 叫醒 | `async function H`，`G(e,0)` | `async function j`，`B(e,0)` | 与 1.126 相同 |

## 3.12.28 这次改了什么

编辑器从 1.126 换到 1.141。普通对话那份 `windsurf-chat-client\index.js` 里不再有模型列表，只剩空的 `useModelCatalog`。列表、点选、图标、倍率和模式都在网页包里。`windsurf-chat-client` 对不上时日志写「这版普通对话和 Agent 共用模型列表」，这不是失败。

| 功能 | 3.12.28 |
|---|---|
| Agent 列表 | `Ur` / `Wr`，文件 `app-initial-DlcIkwcB.js` |
| Agent 排序 | `h=D(()=>n.filter(e=>e!==t),[n,t])` |
| 认编号 / 插入点 | `ma` / `pa`，`function pa(e,t){return zr(e,t)}` |
| 新建会话 | `c[l.id]=ma(l,r,u,a)`，会话编号是 `t` |
| 发出去的值 | `...Pr(a)` 和 `...Pr(n)`，带 `@` 的仍换成 `swe-1-6-slow` |
| 清值 | `I(()=>{n!==null&&c!==n.baseValue&&r(null)},[n,c])` |
| 思考档 | `ie` / `Qo`，列表变量 `g`，当前值 `n` |
| 当前模型 | `{value:u??n?.value??c,onSelect:`，只在 `t.category==="model"` 时套用 |
| 点选 | `te=L(t=>{r(t.modelUid),C(t.modelUid),i(\`LocalModelPicker:Select:Model\`` |
| 发送前再记一次会话 | `u=L(e=>{c(e).then(t=>{t&&l(e)})},[c,l])`，会话编号是 `_o()` 的 `n.sessionId`。漏了这句，后开的会话会把全局模型盖掉 |
| 没选模型不能发送 | 见下面七句。七句必须同时正好出现 1 次，缺一句整组不写 |
| 上下文用量 | `function ed(){let e=_r(e=>e.status.contextUsage)`，组件是 `Hr`。3.11.238 的 `function Dm()` 对不上，圆环会停在替身模型的 200K。分母用编号第三段 |
| 图标 | 函数仍是 `Kn`。选项行用 `J.jsx`。下拉行是 `n=S.icon??(t?Nt(e.modelUid)??Kn(S.provider)`，`Y.jsx` |
| 倍率 | `K.jsx`，组件 `Le` `Dn` `Tn` `On`，文件 `app-initial-B6DX4ee6.js` |
| 叫醒 | `},1e4)}async function V`，目录是 `g.MetadataProvider`，刷新调用 `q(e,0)` |

没选模型的七句，按下面原文搜。七句都要正好 1 次：

```text
按钮文案（排序那份脚本）
$e=Oe.length===0?Ee(`modelSelect.noneSelected`):At(Ze,re,Ee)

模型按钮没挂出来
k&&(0,Z.jsx)(ra,{disabled:ps||F,onClose:Id})

发送箭头
disabled:t,onDisabledPress:s,className:d,menu:_,menuVisible:v,onClick:w?m:i

回车和箭头真正发出去
Pf=K(e=>{Nf()||(cf.onSend(),z.guardSubmit(()=>{zu(!1),Md(e)}))})

排队
Wu=K((e,t=!1)=>{if(Qa)return;

输入框自己补发
oa(!1),zc||Md()

后台发送
fp=K(()=>{Nf()||(cf.onSend(),z.guardSubmit(()=>{zu(!1),Md({background:!0})}))})
```

文案那句在 `app-initial-B6DX4ee6.js`，其余在 `app-initial-F2h9z6FE.js`。哈希每次发版都会变。画出 None selected 时把 `bailanHasModel` 记成假，画出模型名时记成真。发送箭头排在模型按钮后面，同一次绘制读得到。模型按钮这次没渲染，必须把标记改回可以发送，否则上一页的 None selected 会把首页输入框卡住。只改箭头的 `disabled` 不够，回车走的是 `Pf`。排队走 `Wu`。`Wu=K((e,t=!1)=>{` 太短，替换结果仍包含这句开头，还原和再套都会对错。锚点要带到后面的 `if(Qa)return;`。

上下文用量会把 `var bailanLast="";var bailanChat={` 从中拆开，插进 `bailanLast` 的镜像。拆开之后，列表补丁认不出原来的插入整段，会再插一次 `function bailanSyncModel`。文件里已经有这个函数时，跳过插入。出现两次时 Agent 窗口是白的，控制台是 `Identifier 'bailanSyncModel' has already been declared`。

`swe-1-6-slow` 仍在 `windsurf-chat-client\index.js` 里。接口域名没变。脚本仍是散文件。普通对话列表对不上、又看到空的 `useModelCatalog` 时，日志是「这版普通对话和 Agent 共用模型列表」，不是锚点失败。

## 装新版本时

安装包会换掉整个 `resources`。上一版写进脚本里的句子跟着消失，正在跑的 Hub 不会自动再写一遍。补丁只发生在拦截启动、Devin 被关掉之后的那一次 `Restart`。

顺序是：先停拦截（它会还原旧补丁、关掉 Devin、清脚本缓存），再装新版本，看日志里哪几句「对不上」，改完锚点后退出 Hub、重新编译、再开拦截。安装包已经换成干净文件时，不用再点还原，里面已经没有 Hub 的句子。

正在跑的 Hub 用的是编译进去的旧锚点。源码改了不重新编译，开拦截仍按旧句子去对。exe 被占用时编译会失败，先退出再编。

写进去的是 `DiscoverExecutable` 找到的那一个 `Devin.exe`，不是关于窗口里看到的那个名字。查找顺序：设置里的路径、`%LOCALAPPDATA%\Programs\Devin\Devin.exe`、Program Files、正在运行的进程、注册表、各盘的 `\Devin\Devin.exe`。机器上有两份安装时，会改先找到的那份。日志里的 `Devin 已自动重启:` 后面就是这次改的路径。客户版和完整版各有一份 `config.json`，路径不共用。

`Devin.exe` 和 `language_server_*.exe` 都要退出。日志出现「Devin 进程未能完全退出」时，文件没写上，新开的仍是官方脚本。脚本缓存只在至少一处写成功时清除，Devin 必须重新打开才会加载新脚本。

`patchdata` 里新的 txt 必须和安装文件一样用 LF。`picker_after.txt`、`picker_after_126.txt` 现在是 CRLF，还原靠这串字节，不要另存成 LF。编辑器或 git 把 238 的 txt 转成 CRLF 后，句子看起来一样，出现次数会变成 0。

角标补丁只负责把目录里的倍率文字画出来。日志已经写上倍率、界面没有蓝色数字，查 `devincatalog` 的 promo，不查组件名。模型从后台目录消失，是健康检查或渠道同步删的，同样不是 Devin 改名。

`set_session_model` 会同时改掉全局当前模型。某个会话没对上编号时，转发用的就是这个全局模型，也就是上一次成功通知的那个。

新版本第一次开拦截，有几句「对不上」是预期结果。对照日志改那几处，编译一次再开。反复开关不会让对不上的句子自己对上。

再核对四件和压缩名无关的事：

- 发给 Agent 的替身名字是 `swe-1-6-slow`。选择器里不展示它，目录里必须留着，否则 Agent 报没有可用模型，选模型的入口也不出现。新版目录里搜不到这个名字时，同时改 `agentpatch.go` 的 `agentModelAlias` 和 `catalog.go` 的 `agentFallback`。
- 接口仍走 `server.codeium.com`、`server.self-serve.windsurf.com` 这组，目录里的推理地址是 `https://server.codeium.com`。域名换了，列表和对话一起断，要改 `DefaultTargetDomains` 和 `inferenceURL`，不是改脚本里的函数名。
- 要改的脚本仍是散文件：`windsurf-chat-client\index.js`、`devin-webapp-host\assets\app-initial-*.js`、`extensions\windsurf\dist\extension.js`。这三处收进 `app.asar` 后，现在的替换找不到文件。
- 列表补丁对上了却是空的，看过滤字段还是不是 `family.uid` 和 `modelInfo.modelFamilyUid`。字段改名时原文句子仍能对上，滤完没有行。界面仍是旧的，日志却写成功，再清 `%APPDATA%\Devin\CachedData`，并确认 Devin 是这次写盘之后重新打开的。现在还原只删 `Code Cache`。

企业账号和个人账号不影响这些文件。补丁在安装目录里。

## 下一版怎么改

1. 看 `product.json` 的 `windsurfVersion` 和关于里的提交。内部 VS Code 版本相同，不代表脚本没变。
2. 先还原安装目录，或另拷一份还没打补丁的 `resources`。已经打过补丁的文件里搜不到原文。
3. 用下面的检查脚本跑一遍。哪一项是 0，就只改那一项。
4. 在对应的 `*Old` 旁边加一组新版本常量，放进尝试列表的最前面。还原列表同样把新版本放最前，并且还原到这一版自己的原文。
5. 列表这种整段，另存 `patchdata/picker_before_<版本>.txt` 和 `picker_after_<版本>.txt`，`go:embed` 进来。原文保持文件里的样子，包括没有结尾换行。
6. 在 `local-go` 下跑 `go test ./internal/devin/ ./internal/proxysrv/ ./internal/connectframe/`。本机回合测试只对拷出来的目录调用 `apply*` / `restore*`。不要对假路径调用 `ApplyClientPatches`，它会落到真正的 `Devin.exe` 上。`devin238_install_test.go` 写死了 `app-initial-DnnoLajw.js` 和 `app-initial-CECksEiP.js`。`devin312_install_test.go` 写死了 `app-initial-DlcIkwcB.js` 和 `app-initial-B6DX4ee6.js`。哈希一变，这两个测试要一起改。没选模型那组按句子扫描，不写死文件名。
7. 退出正在跑的 Hub（exe 被锁），用 `build_customer.bat` 重新编译客户版再启动拦截。`dist` 里那份加密包不会带上这次源码改动，要另做一次加密。
8. 在 `proxy_日期.log` 里这几句要齐：写进 Agent 列表、排序里可以按分组看、上下文用量按选中的档位显示、对不上的模型用上了 Hub 的图标、没选模型时不能发送、已在模型名字旁边标上倍率、已接上模型变化后叫醒 Devin。3.12.28 没有单独的普通对话列表，对应的是「这版普通对话和 Agent 共用模型列表」。客户版窗口会把其中几句滤掉。用户重开会话和模型列表后再看界面。拦截启动是异步的，等日志里出现 Devin 已重启，再读这一轮结果。新会话不选模型时发送箭头应是灰的，回车也不发出去。选中模型后再发，日志里应出现「按会话」。

日志里「已经写好」「已经接上」表示这台机器上已经是新锚点，不是失败。「对不上，没有改动」才是要补锚点。

## 已经踩过的坑

- 3.11.238 第一次拦截只有叫醒写上了。Agent 的 `tr` 改名成 `wr` 之后，列表、排序、会话三条一起报对不上。叫醒用的仍是 1.126 的 `async function j`。
- 对话文件是一次写盘。图标函数名没补上时，`patchChatSession` 直接返回，列表即使对上了也不会落盘。
- 图标改了 `ModelSelectOptionItem` 之后，IDE 有图，Agent 下拉仍没图。用户看见的是 `n=C.icon??` 那一行。
- 给 `bailanSrc` 传入带 `provider` 的对象，函数直接返回空。未知厂商不会去走前缀匹配。
- 模型和模式共用 `_o`。把带 `@` 的模型编号填进模式，按钮变成 Mode。
- 排序补丁把「替换结果已经出现」当成整目录都写好了。替换结果若在别的文件里碰巧存在，真正那一处会被跳过。
- 从已打补丁的安装里摘原文，摘到的是 Hub 改过的句子。先还原，或从更新包里拿干净文件。
- 还原叫醒后如果又开了拦截，叫醒会重新写上。确认干净时，在 `resources` 里搜 `bailan`、`127.0.0.1:5099`、`devinhub-family`，三处都应为 0。
- 界面改了要重编译。`//go:embed` 的页面和这些补丁都在 exe 里，正在跑的进程不会自己变。
- 对话那一份是一次写盘。列表对上之后，点选、排序、图标、当前模型任一处出现次数不是 1，函数会在 `WriteFile` 之前返回，内存里已经替换的列表也丢掉。日志只剩一句「对不上」。
- 「已把你的模型写进 Agent」只说明摊平那句写上了。新建会话和模式保护要再搜 `set_session_model`、`category==="model"`。
- 通知地址写死 5099。Hub 改听别的端口时，补丁看起来成功，点选却没有 `set_model`。
- 上下文用量先把 `var bailanLast="";var bailanChat={` 拆开，再开一次拦截，会把 `function bailanSyncModel` 插进两遍。Agent 窗口空白，渲染日志是 `Identifier 'bailanSyncModel' has already been declared`。文件里已经有这个函数时不要再插。
- 3.12.28 的圆环不在原来的 `function Dm()` 里，改到了 `function ed()`，提示文字是 `% (已用 / 上限) context used`。还按 `Dm` 去配时，1M 的模型仍显示 200K。
- 在 gzip 的 Connect 帧上直接搜会话名，次数是 0。先 `MessagePayload` 解压。客户版日志不保留「请求里的会话编号」，字段对不上时看 `[转发对照]`。正文里有两个不同模型的会话名时不猜。
- 3.12.28 的 `windsurf-chat-client\index.js` 没有 `Available models`。日志「共用模型列表」不是失败，不要为了这句话去改对话列表锚点。
- 没选模型只改了箭头的禁用，回车仍走 `Pf`，排队仍走 `Wu`。`Wu` 的锚点如果停在 `{`，替换结果里还含着原文，再套一次会把判断写两遍。
- 模型按钮没渲染时不把 `bailanHasModel` 改回可以发送，首页或别的输入框会被上一页的 None selected 卡住。

## 检查脚本

在干净的 `resources` 上跑。数字不是 1，就是这版要改的锚点。反引号和路径按自己的安装改。

```python
from pathlib import Path

root = Path(r"E:\Devin\resources")
chat = (root / r"app\out\vs\workbench\windsurf-chat-client\index.js").read_text(encoding="utf-8", errors="ignore")
ext = (root / r"app\extensions\windsurf\dist\extension.js").read_text(encoding="utf-8", errors="ignore")
assets = root / r"app\out\vs\workbench\devin-webapp-host\assets"
agent = "\n".join(p.read_text(encoding="utf-8", errors="ignore") for p in assets.glob("app-initial-*.js"))

needles = {
    "对话列表 Available models": (chat, "Available models"),
    "对话点选": (chat, "pendingSwitchTarget"),
    "对话当前模型": (chat, "activeSessionId:"),
    "对话图标行": (chat, 'size-[18px] shrink-0 items-center justify-center'),
    "对话倍率": (chat, "fastStatus?.isActive"),
    "叫醒": (ext, "},1e4)}async function"),
    "Agent 列表": (agent, "models.flatMap"),
    "Agent 点选": (agent, "LocalModelPicker:Select:Model"),
    "Agent 回放": (agent, "session/set_config_option"),
    "Agent 排序": (agent, "modelSelect.orderBy"),
    "Agent 下拉图标": (agent, "C.icon??"),
    "模式": (agent, 'id==="mode"'),
    "没选模型文案": (agent, "modelSelect.noneSelected"),
    "发送": (agent, "Pf=K(e=>{"),
    "上下文圆环": (agent, "e.status.contextUsage"),
}
for name, (text, needle) in needles.items():
    print(f"{text.count(needle):4}  {name}")
```

某一项要从 1 扩成唯一片段时，用同一段 Python 打印 `count`，不要用 PowerShell 的 here-string。
