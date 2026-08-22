介绍地址

https://docs.cline.bot/getting-started/clinepass

clinepass 没有opencode go那样的单模型限额，所以需要把这个相关的功能去掉


入门
线通行证
每月只需​​支付少量费用即可订阅，在流行的开源编码模型上，其使用量是标准 API 费率的 2-5 倍。

ClinePass 是一项低成本的月度订阅服务——每月 9.99 美元——与标准 API 费率相比，在流行的开源编码模型上可提供2-5 倍的使用量。
完全是可选的。您无需 ClinePass 即可使用 Cline，而且您也可以同时使用任何其他服务提供商。
订阅 ClinePass
每月只需​​ 9.99 美元即可开始使用，与标准 API 费率相比，在流行的开源编码模型上可获得 2-5 倍的使用量。
​
工作原理
ClinePass是Cline中的一个独立服务提供商。订阅后，在配置服务提供商时，请选择ClinePass 。
IDE 扩展：转到 IDE 扩展设置，将API 提供程序设置为ClinePass，然后登录。
Cline IDE 扩展设置，已选择 ClinePass 作为 API 提供程序
CLI：前往/settings并选择ClinePass作为您的提供商。
Cline CLI 设置显示已选择 ClinePass 作为提供商
​
为什么选择 ClinePass
开放模型已经发展得非常出色。它们在编码任务方面的性能现在已经接近专有模型，而且由于许多提供商都能以具有竞争力的价格提供服务，因此它们通常要便宜得多。
然而，获得可靠的访问权限可能很困难。服务提供商的质量和可用性参差不齐，而且标准的速率限制可能会限制读取文件、运行命令以及跨多个回合迭代的大型代理工作流程。
ClinePass 通过以下方式解决这个问题：
精心挑选一组经过测试和基准测试的开源模型，用于编码代理。
与标准 API 费率相比，在流行的开源编码模型上可提供2-5 倍的使用率，因此您可以运行长时间、复杂的代理任务而不会中断。
通过 Cline 的基础设施提供稳定的访问
ClinePass 与 Cline（按使用量计费）是两个独立的服务提供商。您可以单独使用两者——订阅 ClinePass，即可在常用的开源编码模式下享受比标准 API 费率高 2-5 倍的使用量；或者使​​用 Cline（按使用量计费）进行按需付费访问。
​
模型
ClinePass 包含以下模型，这些模型均经过测试和基准测试，适用于编码代理：
模型	型号 ID
GLM-5.3	cline-pass/glm-5.3
GLM-5.2	cline-pass/glm-5.2
Kimi K3	cline-pass/kimi-k3
Kimi K2.7 代码	cline-pass/kimi-k2.7-code
Kimi K2.6	cline-pass/kimi-k2.6
DeepSeek V4 Pro	cline-pass/deepseek-v4-pro
DeepSeek V4 闪存	cline-pass/deepseek-v4-flash
MiMo-V2.5	cline-pass/mimo-v2.5
MiMo-V2.5-Pro	cline-pass/mimo-v2.5-pro
MiniMax M3	cline-pass/minimax-m3
Qwen3.8 Max	cline-pass/qwen3.8-max
Qwen3.7 Max	cline-pass/qwen3.7-max
Qwen3.7 Plus	cline-pass/qwen3.7-plus
​
在 Cline 之外使用 ClinePass
您可以通过 Cline API 在自己的脚本、应用程序或自动化流程中使用 ClinePass 模型。该 API 使用与 Cline 其他 API 相同的 OpenAI 兼容的聊天补全格式。
首先，请在app.cline.bot的“设置”>“API 密钥”中创建 API 密钥。有关完整步骤，请参阅Cline API 入门指南。
现场请使用完整的 ClinePass 模型代码model：
export CLINE_API_KEY="your_api_key_here"

curl -X POST https://api.cline.bot/api/v1/chat/completions \
  -H "Authorization: Bearer $CLINE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "cline-pass/qwen3.7-max",
    "messages": [
      {"role": "user", "content": "Write a TypeScript function that validates an email address."}
    ],
    "stream": false
  }'
​
参考定价
ClinePass 采用固定月费订阅模式，因此您无需支付下方列出的单个 API 价格。这些参考价格显示的是每种模型每百万代币的底层费率，可以帮助您了解 ClinePass 配额的使用情况（配额的 2-5 倍相当于支付标准 API 费率）。
模型	输入	输出	缓存读取	缓存写入
GLM-5.3	1.40美元	4.40美元	0.26美元	-
GLM-5.2	1.40美元	4.40美元	0.26美元	-
Kimi K3	3.00美元	15.00美元	0.30美元	-
Kimi K2.7 代码	0.95美元	4.00美元	0.19美元	-
Kimi K2.6	0.95美元	4.00美元	0.16美元	-
DeepSeek V4 Pro（峰值）1	1.32美元	3.96美元	0.044美元	-
DeepSeek V4 Pro（非高峰时段）1	0.66美元	1.98美元	0.022美元	-
DeepSeek V4 Flash (Peak) 1	0.44美元	1.32美元	0.014美元	-
DeepSeek V4 闪光灯（非高峰时段）1	0.22美元	0.66美元	0.007美元	-
MiMo-V2.5	0.14美元	0.28美元	0.0028美元	-
MiMo-V2.5-Pro	1.74美元	3.48美元	0.0145美元	-
MiniMax M3	0.30美元	1.20美元	0.06美元	-
Qwen3.8 Max	2.00美元	6.00美元	0.25美元	2.50美元
Qwen3.7 Max	2.50美元	7.50美元	0.50美元	3.125美元
Qwen3.7 Plus（≤ 256K 代币）	0.40美元	1.60美元	0.04美元	0.50美元
Qwen3.7 Plus（> 256K 代币）	1.20美元	4.80美元	0.12美元	1.50美元

