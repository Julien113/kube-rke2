import (
    "sigs.k8s.io/e2e-framework/pkg/env"
    conf "sigs.k8s.io/e2e-framework/klient/conf"
)

var (
    global env.Environment
)

func TestMain(m *testing.M) {
	global = env.New()    
    global.Setup(func(context.Context, cfg envconf.Config) (context.Context, error){
        // code to setup environment
        return nil, nil
    })
    os.Exit(global.Run(m))
}