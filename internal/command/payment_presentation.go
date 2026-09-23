package command

import (
	"fmt"
	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/agentenv"
)

// paymentPresentationHint adds invocation context without duplicating host rules.
func paymentPresentationHint(getenv func(string) string, hosted bool) string {
	return fmt.Sprintf("当前环境标记：%s；本次是否有匿名托管支付入口：%t。按以下共享指引及实际返回字段展示。\n%s", agentenv.Detect(getenv), hosted, cliembed.PaymentPresentationGuide())
}
