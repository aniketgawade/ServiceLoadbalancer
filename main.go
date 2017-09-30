package main

import (
	"fmt"
	"os"
	"flag"
	"bytes"
	"strings"
	"github.com/aniketgawade/contrail-go-api"
	"github.com/aniketgawade/contrail-go-api/config"
	"github.com/aniketgawade/contrail-go-api/types"
	docker_types "github.com/docker/docker/api/types"
	docker_client "github.com/docker/docker/client"
	"golang.org/x/net/context"
	"github.com/davecgh/go-spew/spew"
	"text/template"
)


var (
	/*
	 * OpenContrail API server
	 */
	oc_client *contrail.Client
	oc_server string
	oc_port   int

	/*
	 * Authentication
	 */
	os_tenant_name string
	os_username    string
	os_password    string
)

const networkShowDetail = `  Network: {{.Name}}
      Uuid: {{.Uuid}}        State: {{if .AdminState}}UP{{else}}DOWN{{end}}
      NetwordId: {{.NetworkId | printf "%-5d"}}    Mode: {{.Mode}}    Transit: {{.Transit}}
      Subnets: {{range .Subnets}}{{.}} {{end}}{{if .RouteTargets}}
      RouteTargets: {{range .RouteTargets}}{{.}} {{end}}{{end}}
      {{if .Policies}}Policies:{{end}}{{range .Policies}}
         {{.}}
      {{end}}
`

func networkList(client *contrail.Client) {
	var parent_id string
	var err error
	parent_id, err = config.GetProjectId(
		client, "default-domain:default-project", "")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	networkList, err := config.NetworkList(client, parent_id, true)
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	var tmpl string
	tmpl = networkShowDetail
	t := template.Must(template.New("network-list").Parse(tmpl))
	for _, n := range networkList {
		t.Execute(os.Stdout, n)
	}
}

func usage() {
	flag.PrintDefaults()
}

func InitFlags() {
	flag.StringVar(&oc_server, "server", "localhost",
		"OpenContrail API server hostname or address")
	flag.IntVar(&oc_port, "port", 8082,
		"OpenContrail API server port")
	flag.StringVar(&os_tenant_name,
		"os-tenant-name", os.Getenv("OS_TENANT_NAME"),
		"Authentication tenant name (Env: OS_TENANT_NAME)")
	flag.StringVar(&os_username,
		"os-username", os.Getenv("OS_USERNAME"),
		"Authentication username (Env: OS_USERNAME)")
	flag.StringVar(&os_password,
		"os-password", os.Getenv("OS_PASSWORD"),
		"Authentication password (Env: OS_PASSWORD)")
}

func CreateLoadBalancer(name string) {
	projectId, err := config.GetProjectId(
		oc_client, os_tenant_name, "")
	if err != nil {
		fmt.Printf("Error in finding project by uuid\n")
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	project_obj, err := oc_client.FindByUuid("project", projectId)
	if err != nil {
		fmt.Printf("Error in finding project obj\n")
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	//Find if loadbalancer already exist
	var fqn []string
	fqn = append(fqn, "default-domain")
	fqn = append(fqn, os_tenant_name)
	fqn = append(fqn, name)
	interface_obj, err := types.VirtualMachineInterfaceByName(oc_client , strings.Join(fqn, ":"))
	instance_ip_obj, err := types.InstanceIpByName(oc_client , strings.Join(fqn, ":"))
	lb_instance, err := types.LoadbalancerByName(oc_client , strings.Join(fqn, ":"))
	if lb_instance == nil {
		fmt.Println("Now creating loadbalancer")
		lb_instance = new(types.Loadbalancer)
		lb_instance.SetParent(project_obj)
		lb_instance.SetName(name)
		lb_instance.SetDisplayName(name)
		lb_instance.SetLoadbalancerProvider("native")
		//id_perms := new(types.IdPermsType{Enable: true})
		props := new(types.LoadbalancerType)
		props.ProvisioningStatus = "ACTIVE"
		props.OperatingStatus = "ONLINE"
		props.VipAddress = "114.11.11.9"

		lb_instance.SetLoadbalancerProperties(props)
		//fmt.Printf("%v", lb_instance)	
		//Creating native load balancer 
		var networkFQName bytes.Buffer
		networkFQName.WriteString("default-domain:")
		networkFQName.WriteString(os_tenant_name)
		networkFQName.WriteString(":")
		networkFQName.WriteString("y0u1enhesubxldynvi8bdquqc")
		fmt.Printf("----->> %s\n", networkFQName.String())
		networkObj, err := types.VirtualNetworkByName(oc_client, networkFQName.String())
		if interface_obj == nil {
			interface_obj = new(types.VirtualMachineInterface)
			interface_obj.SetName(name)
			interface_obj.SetParent(project_obj)

			if err != nil {
				fmt.Printf("Error in finding network\n")
				fmt.Fprint(os.Stderr, err)
				os.Exit(1)
			}
			err = interface_obj.AddVirtualNetwork(networkObj)
			if err != nil {
				fmt.Printf("Error in adding network\n")
				fmt.Fprint(os.Stderr, err)
				os.Exit(1)
			}
			err = oc_client.Create(interface_obj)
			if err != nil {
				fmt.Printf("Error in creating interface\n")
				fmt.Fprint(os.Stderr, err)
				os.Exit(1)
			}
		}
		if instance_ip_obj == nil {
			instance_ip_obj = new(types.InstanceIp)
			instance_ip_obj.SetName(name)
			instance_ip_obj.SetInstanceIpAddress("114.11.11.9")

			instance_ip_obj.AddVirtualNetwork(networkObj)
			instance_ip_obj.AddVirtualMachineInterface(interface_obj)
			oc_client.Create(instance_ip_obj)
		}
		lb_instance.AddVirtualMachineInterface(interface_obj)
		err = oc_client.Create(lb_instance)
		if err != nil {
			fmt.Printf("Error in creating loadbalancing instance\n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Loadbalancer : %s", lb_instance.GetName())
		if err != nil {
			fmt.Printf("Error in finding loadbalancer\n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
	}
	//Find if loadbalancer listener already exist
	var fqn_lbl []string
	fqn_lbl = append(fqn_lbl, "default-domain")
	fqn_lbl = append(fqn_lbl, os_tenant_name)
	lbl_slice_name := [] string{name, "listener"}
	lbl_name := strings.Join(lbl_slice_name, "_")
	fqn_lbl = append(fqn_lbl, lbl_name)
	lbl_instance, err := types.LoadbalancerListenerByName(oc_client , strings.Join(fqn_lbl, ":"))
	if lbl_instance == nil {
		fmt.Println("Now creating loadbalancing listener")
		lbl_instance = new(types.LoadbalancerListener)
		lbl_instance.SetParent(project_obj)
		lbl_instance.SetName(lbl_name)
		lbl_instance.SetDisplayName(lbl_name)
		listener_prop := new(types.LoadbalancerListenerType)
		listener_prop.Protocol = "HTTP"
		listener_prop.ProtocolPort = 80
		lbl_instance.SetLoadbalancerListenerProperties(listener_prop)
		lbl_instance.AddLoadbalancer(lb_instance)
		err = oc_client.Create(lbl_instance)
		if err != nil {
			fmt.Printf("Error in creating loadbalancing listener instance\n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Loadbalancer  listener : %s", lbl_instance.GetName())
		if err != nil {
			fmt.Printf("Error in finding loadbalancer listener \n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
	}

	var fqn_lbpool []string
	fqn_lbpool = append(fqn_lbpool, "default-domain")
	fqn_lbpool = append(fqn_lbpool, os_tenant_name)
	lbpool_slice_name := [] string{name, "pool"}
	lbpool_name := strings.Join(lbpool_slice_name, "_")
	fqn_lbpool = append(fqn_lbpool, lbpool_name)
	lbpool_instance, err := types.LoadbalancerPoolByName(oc_client , strings.Join(fqn_lbpool, ":"))
	if lbpool_instance == nil {
		fmt.Println("Now creating loadbalancing pool")
		lbpool_instance = new(types.LoadbalancerPool)
		lbpool_instance.SetParent(project_obj)
		lbpool_instance.SetName(lbpool_name)
		lbpool_instance.SetDisplayName(lbpool_name)
		lbpool_prop := new(types.LoadbalancerPoolType)
		lbpool_prop.Protocol = "HTTP"
		lbpool_prop.LoadbalancerMethod = "ROUND_ROBIN"
		lbpool_instance.SetLoadbalancerPoolProperties(lbpool_prop)
		lbpool_instance.AddLoadbalancerListener(lbl_instance)
		err = oc_client.Create(lbpool_instance)
		if err != nil {
			fmt.Printf("Error in creating loadbalancing pool instance\n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Loadbalancer  pool : %s", lbpool_instance.GetName())
		if err != nil {
			fmt.Printf("Error in finding loadbalancer pool \n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
	}

	var fqn_lbmemb []string
	fqn_lbmemb = append(fqn_lbmemb, "default-domain")
	fqn_lbmemb = append(fqn_lbmemb, os_tenant_name)
	fqn_lbmemb = append(fqn_lbmemb, lbpool_name)
	lbmemb_slice_name := [] string{name, "member"}
	lbmemb_name := strings.Join(lbmemb_slice_name, "_")
	fqn_lbmemb = append(fqn_lbmemb, lbmemb_name)
	lbmemb_instance, err := types.LoadbalancerMemberByName(oc_client , strings.Join(fqn_lbmemb, ":"))
	if lbmemb_instance == nil {
		fmt.Println("Now creating loadbalancing member")
		lbmemb_instance = new(types.LoadbalancerMember)
		lbmemb_instance.SetParent(lbpool_instance)
		lbmemb_instance.SetName(lbmemb_name)
		lbmemb_instance.SetDisplayName(lbmemb_name)
		member_prop := new(types.LoadbalancerMemberType)
		member_prop.Address = "114.11.11.3"
		member_prop.ProtocolPort = 80
		lbmemb_instance.SetLoadbalancerMemberProperties(member_prop)
		err = oc_client.Create(lbmemb_instance)
		if err != nil {
			fmt.Printf("Error in creating loadbalancing memeber instance\n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Loadbalancer  listener : %s", lbmemb_instance.GetName())
		if err != nil {
			fmt.Printf("Error in finding loadbalancer member \n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
	}
	if true {
		fmt.Println("Deleting everything")
		oc_client.Delete(instance_ip_obj)
		oc_client.Delete(interface_obj)
		oc_client.Delete(lbmemb_instance)
		oc_client.Delete(lbpool_instance)
		oc_client.Delete(lbl_instance)
		oc_client.Delete(lb_instance)
	}
}
func main() {
	InitFlags()
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() < 1 {
		usage()
		os.Exit(2)
	}

	oc_client = contrail.NewClient(oc_server, oc_port)
	//networkList(oc_client)

	service_name := flag.Arg(0)
	fmt.Println("Loadbalancing service name: ", service_name)
	cli, err := docker_client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	services, err := cli.ServiceList(context.Background(), docker_types.ServiceListOptions{})
	if err != nil {
		panic(err)
	}

	for _, service := range services {
		//spew.Dump(service)
		fmt.Println("->>>>", service.Spec.Annotations.Name)
		fmt.Println("->>>>", service.Endpoint.VirtualIPs[0].Addr)
	}
	containers, err := cli.ContainerList(context.Background(), docker_types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	for _, container := range containers {
		spew.Dump(container.Labels)
		fmt.Println(container.ID)
		for label, value := range container.Labels {
	 		if (label == "com.docker.swarm.service.name") {
				fmt.Println("->>>>", value)
			}
		}
	}

	CreateLoadBalancer(service_name)
}
