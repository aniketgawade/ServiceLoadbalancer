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
	"github.com/Pallinder/go-randomdata"
	docker_types "github.com/docker/docker/api/types"
	docker_client "github.com/docker/docker/client"
	"golang.org/x/net/context"
	//"github.com/davecgh/go-spew/spew"
	"text/template"
)

var delete_all = false
var (
	/*
	 *  Docker API
	 */
	docker_cli *docker_client.Client
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
	
	/*
	 * Service defaults
	 */
	service_name   	    string
	fip_pool_arg 	    string
	service_port        int
	service_protocol    string
	overwrite_vip	    string
	lb_algo 	    string
	delete_lb	    bool 
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
	flag.StringVar(&service_name, "service-name", "",
		"Load balance service")
	flag.IntVar(&service_port, "protocol-port", 80,
		"Load balance service port")
	flag.StringVar(&service_protocol, "protocol", "HTTP",
		"Load balance service protocol")
	flag.StringVar(&overwrite_vip, "overwrite-vip", "",
		"Load balancer overwrite vip ")
	flag.StringVar(&lb_algo, "algo", "ROUND_ROBIN",
		"Load balancer algorithm")
	flag.BoolVar(&delete_lb, "delete", false,
		"Load balancer delete obj")
	flag.StringVar(&fip_pool_arg, "floating-ip", "",
		"Floating ip for vip")
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
	//interface_obj, err := types.VirtualMachineInterfaceByName(oc_client , strings.Join(fqn, ":"))
	//instance_ip_obj, err := types.InstanceIpByName(oc_client , strings.Join(fqn, ":"))
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
		if len(overwrite_vip) != 0 {
			fmt.Printf("Switching to new vip : %s", overwrite_vip)
			props.VipAddress = overwrite_vip
		} else {
			props.VipAddress = getDockerServiceVip(name)
		}
		var networkFQName bytes.Buffer
		networkFQName.WriteString("default-domain:")
		networkFQName.WriteString(os_tenant_name)
		networkFQName.WriteString(":")
		networkFQName.WriteString(getDockerServiceNetwork(name))
		fmt.Printf("----->> %s\n", networkFQName.String())
		networkObj, err := types.VirtualNetworkByName(oc_client, networkFQName.String())
		props.VipSubnetId = getSubnetFromNetwork(networkObj)
		props.AdminState = true
		lb_instance.SetLoadbalancerProperties(props)
		//if interface_obj == nil {
			interface_obj := new(types.VirtualMachineInterface)
			interface_obj.SetName(name + "_" + randomdata.SillyName())
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
		//}
		//if instance_ip_obj == nil {
			instance_ip_obj := new(types.InstanceIp)
			instance_ip_obj.SetName(name + "_" + randomdata.SillyName())
			if len(overwrite_vip) != 0 {
				fmt.Printf("Switching to new vip : %s", overwrite_vip)
				instance_ip_obj.SetInstanceIpAddress(overwrite_vip)
			} else {
				instance_ip_obj.SetInstanceIpAddress(getDockerServiceVip(name))
			}

			instance_ip_obj.AddVirtualNetwork(networkObj)
			instance_ip_obj.AddVirtualMachineInterface(interface_obj)
			err = oc_client.Create(instance_ip_obj)
			if err != nil {
				fmt.Printf("Error in creating instance ip\n")
				fmt.Fprint(os.Stderr, err)
				os.Exit(1)
			}
		//}
		if len(fip_pool_arg) != 0 {
			addFloatingIp(interface_obj.GetUuid(), fip_pool_arg)
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
		listener_prop.Protocol = service_protocol
		listener_prop.ProtocolPort = service_port
		listener_prop.AdminState = true
		listener_prop.ConnectionLimit = 100
		listener_prop.DefaultTlsContainer = "EMPTY"
		listener_prop.SniContainers = ([]string{"EMPTY"})
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
		lbpool_prop.Protocol = service_protocol
		lbpool_prop.LoadbalancerMethod = lb_algo
		var networkFQName bytes.Buffer
		networkFQName.WriteString("default-domain:")
		networkFQName.WriteString(os_tenant_name)
		networkFQName.WriteString(":")
		networkFQName.WriteString(getDockerServiceNetwork(name))
		//fmt.Printf("----->> %s\n", networkFQName.String())
		networkObj, err := types.VirtualNetworkByName(oc_client, networkFQName.String())
		lbpool_prop.SubnetId = getSubnetFromNetwork(networkObj)
		lbpool_prop.AdminState = true
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
	
	for lb_container, container_ip := range getAllContainersIpInService(name) {	
		var fqn_lbmemb []string
		fqn_lbmemb = append(fqn_lbmemb, "default-domain")
		fqn_lbmemb = append(fqn_lbmemb, os_tenant_name)
		fqn_lbmemb = append(fqn_lbmemb, lbpool_name)
		lbmemb_slice_name := [] string{lb_container, "member"}
		lbmemb_name := strings.Join(lbmemb_slice_name, "_")
		fqn_lbmemb = append(fqn_lbmemb, lbmemb_name)
		lbmemb_instance, err := types.LoadbalancerMemberByName(oc_client , strings.Join(fqn_lbmemb, ":"))
		if lbmemb_instance == nil {
			fmt.Println("Now creating loadbalancing member")
			lbmemb_instance = new(types.LoadbalancerMember)
			lbmemb_instance.SetParent(lbpool_instance)
			lbmemb_instance.SetName(lbmemb_name)
			lbmemb_instance.SetDisplayName(lbmemb_name)
			endpointId := getEndpointFromContainerId(lb_container)
			var vmiFQName bytes.Buffer
			vmiFQName.WriteString("default-domain:")
			vmiFQName.WriteString(os_tenant_name)
			vmiFQName.WriteString(":")
			vmiFQName.WriteString(endpointId)
			vmiObj, err := types.VirtualMachineInterfaceByName(oc_client, vmiFQName.String())
			fmt.Printf("VMI----->> %s\n", vmiObj.GetUuid())
			memKeyValuePair := new(types.KeyValuePairs)
			memKeyValue := new(types.KeyValuePair)
			memKeyValue.Key = "vmi"
			memKeyValue.Value = vmiObj.GetUuid()
			memKeyValuePair.AddKeyValuePair(memKeyValue)
			lbmemb_instance.SetAnnotations(memKeyValuePair)
			member_prop := new(types.LoadbalancerMemberType)
			member_prop.Address = container_ip
			member_prop.ProtocolPort = service_port
			member_prop.Status = "ACTIVE"
			member_prop.AdminState = true
			member_prop.Weight = 1
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
	}
	if delete_all {
		fmt.Println("Deleting everything")
		vmi_refs, err := lb_instance.GetVirtualMachineInterfaceRefs()
		if err != nil {
			fmt.Printf("Error in retriving vmi from load balancer %s: %v", lb_instance.GetUuid(), err)
			return
		}
		for lb_container, _ := range getAllContainersIpInService(name) {	
			var fqn_lbmemb []string
			fqn_lbmemb = append(fqn_lbmemb, "default-domain")
			fqn_lbmemb = append(fqn_lbmemb, os_tenant_name)
			fqn_lbmemb = append(fqn_lbmemb, lbpool_name)
			lbmemb_slice_name := [] string{lb_container, "member"}
			lbmemb_name := strings.Join(lbmemb_slice_name, "_")
			fqn_lbmemb = append(fqn_lbmemb, lbmemb_name)
			lbmemb_instance, _ := types.LoadbalancerMemberByName(oc_client , strings.Join(fqn_lbmemb, ":"))
			err = oc_client.Delete(lbmemb_instance)
			if err != nil {
				fmt.Printf("Error in deleting lb member\n")
				fmt.Fprint(os.Stderr, err)
				os.Exit(1)
			}
		}
		err = oc_client.Delete(lbpool_instance)
		if err != nil {
			fmt.Printf("Error in deleting lb pool\n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
		err = oc_client.Delete(lbl_instance)
		if err != nil {
			fmt.Printf("Error in deleting lb listener instance\n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
		err = oc_client.Delete(lb_instance)
		if err != nil {
			fmt.Printf("Error in deleting lb instance \n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
		for _, vmi_ref := range vmi_refs {
			virtual_machine_interface_obj, err := oc_client.FindByUuid("virtual-machine-interface", vmi_ref.Uuid)
			iip_refs, err := virtual_machine_interface_obj.(*types.VirtualMachineInterface).GetInstanceIpBackRefs()
			if err != nil {
		    		fmt.Printf("Error in retriving iip from vmi %s: %v", virtual_machine_interface_obj.GetUuid(), err)
		    		return
			}
			for _, iip_ref := range iip_refs {
				iip_obj, err := oc_client.FindByUuid("instance-ip", iip_ref.Uuid)
				floating_obj, _ := iip_obj.(*types.InstanceIp).GetFloatingIps()
				err = oc_client.DeleteByUuid("floating-ip",floating_obj[0].Uuid)
		    		err = oc_client.DeleteByUuid("instance-ip", iip_ref.Uuid)
				if err != nil {
					fmt.Printf("Error in  deleteing instance ip\n")
					fmt.Fprint(os.Stderr, err)
					os.Exit(1)
				}
			}
			err = oc_client.DeleteByUuid("virtual-machine-interface", vmi_ref.Uuid)
			if err != nil {
				fmt.Printf("Error in deleting vmi\n")
				fmt.Fprint(os.Stderr, err)
				os.Exit(1)
			}
		}
	}
}


func DeleteLoadBalancer(name string) {
	//Find if loadbalancer already exist
	var fqn []string
	fqn = append(fqn, "default-domain")
	fqn = append(fqn, os_tenant_name)
	fqn = append(fqn, name)
	lb_instance, _ := types.LoadbalancerByName(oc_client , strings.Join(fqn, ":"))
	
	//Find if loadbalancer listener already exist
	var fqn_lbl []string
	fqn_lbl = append(fqn_lbl, "default-domain")
	fqn_lbl = append(fqn_lbl, os_tenant_name)
	lbl_slice_name := [] string{name, "listener"}
	lbl_name := strings.Join(lbl_slice_name, "_")
	fqn_lbl = append(fqn_lbl, lbl_name)
	lbl_instance, _ := types.LoadbalancerListenerByName(oc_client , strings.Join(fqn_lbl, ":"))

	var fqn_lbpool []string
	fqn_lbpool = append(fqn_lbpool, "default-domain")
	fqn_lbpool = append(fqn_lbpool, os_tenant_name)
	lbpool_slice_name := [] string{name, "pool"}
	lbpool_name := strings.Join(lbpool_slice_name, "_")
	fqn_lbpool = append(fqn_lbpool, lbpool_name)
	lbpool_instance, _ := types.LoadbalancerPoolByName(oc_client , strings.Join(fqn_lbpool, ":"))
	
	if true {
		fmt.Println("Deleting everything")
		vmi_refs, err := lb_instance.GetVirtualMachineInterfaceRefs()
		if err != nil {
			fmt.Printf("Error in retriving vmi from load balancer %s: %v", lb_instance.GetUuid(), err)
			return
		}
		for lb_container, _ := range getAllContainersIpInService(name) {	
			var fqn_lbmemb []string
			fqn_lbmemb = append(fqn_lbmemb, "default-domain")
			fqn_lbmemb = append(fqn_lbmemb, os_tenant_name)
			fqn_lbmemb = append(fqn_lbmemb, lbpool_name)
			lbmemb_slice_name := [] string{lb_container, "member"}
			lbmemb_name := strings.Join(lbmemb_slice_name, "_")
			fqn_lbmemb = append(fqn_lbmemb, lbmemb_name)
			lbmemb_instance, _ := types.LoadbalancerMemberByName(oc_client , strings.Join(fqn_lbmemb, ":"))
			err = oc_client.Delete(lbmemb_instance)
			if err != nil {
				fmt.Printf("Error in deleting lb member\n")
				fmt.Fprint(os.Stderr, err)
				os.Exit(1)
			}
		}
		err = oc_client.Delete(lbpool_instance)
		if err != nil {
			fmt.Printf("Error in deleting lb pool\n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
		err = oc_client.Delete(lbl_instance)
		if err != nil {
			fmt.Printf("Error in deleting lb listener instance\n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
		err = oc_client.Delete(lb_instance)
		if err != nil {
			fmt.Printf("Error in deleting lb instance \n")
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
		for _, vmi_ref := range vmi_refs {
			virtual_machine_interface_obj, err := oc_client.FindByUuid("virtual-machine-interface", vmi_ref.Uuid)
			float_refs, err := virtual_machine_interface_obj.(*types.VirtualMachineInterface).GetFloatingIpBackRefs()
			if err != nil {
		    		fmt.Printf("Error in retriving float-ip from vmi %s: %v", virtual_machine_interface_obj.GetUuid(), err)
		    		return
			}
			for _, float_ref := range float_refs {
				err := oc_client.DeleteByUuid("floating-ip", float_ref.Uuid)
				if err != nil {
					fmt.Printf("Error in deleting floating ip\n")
					fmt.Fprint(os.Stderr, err)
					os.Exit(1)
				}
			}
			iip_refs, err := virtual_machine_interface_obj.(*types.VirtualMachineInterface).GetInstanceIpBackRefs()
			if err != nil {
		    		fmt.Printf("Error in retriving iip from vmi %s: %v", virtual_machine_interface_obj.GetUuid(), err)
		    		return
			}
			for _, iip_ref := range iip_refs {
				iip_obj, err := oc_client.FindByUuid("instance-ip", iip_ref.Uuid)
				floating_obj, err := iip_obj.(*types.InstanceIp).GetFloatingIps()
				if floating_obj != nil {
					err = oc_client.DeleteByUuid("floating-ip",floating_obj[0].Uuid)
					if err != nil {
						fmt.Printf("Error in deleting floating ip\n")
						fmt.Fprint(os.Stderr, err)
						os.Exit(1)
					}
				}
		    		err = oc_client.DeleteByUuid("instance-ip", iip_ref.Uuid)
				if err != nil {
					fmt.Printf("Error in deleteing instance ip\n")
					fmt.Fprint(os.Stderr, err)
					os.Exit(1)
				}
			}
			err = oc_client.DeleteByUuid("virtual-machine-interface", vmi_ref.Uuid)
			if err != nil {
				fmt.Printf("Error in deleting vmi\n")
				fmt.Fprint(os.Stderr, err)
				os.Exit(1)
			}
		}
	}
}

func getDockerServiceNetwork(service_name string) string {
	services, err := docker_cli.ServiceList(context.Background(), docker_types.ServiceListOptions{})
	if err != nil {
		panic(err)
	}

	for _, service := range services {
		//spew.Dump(service)
		if strings.Compare(service.Spec.Annotations.Name, service_name) == 0 {
			fmt.Println("->>>>", service.Spec.Annotations.Name)
			fmt.Println("->>>>", service.Endpoint.VirtualIPs[0].Addr)
			return service.Endpoint.VirtualIPs[0].NetworkID
		}
	}
	return "ERR"
}
func getDockerServiceVip(service_name string) string {
	services, err := docker_cli.ServiceList(context.Background(), docker_types.ServiceListOptions{})
	if err != nil {
		panic(err)
	}

	for _, service := range services {
		//spew.Dump(service)
		if strings.Compare(service.Spec.Annotations.Name, service_name) == 0 {
			fmt.Println("->>>>", service.Spec.Annotations.Name)
			fmt.Println("->>>>", strings.Split(service.Endpoint.VirtualIPs[0].Addr, "/")[0])
			return strings.Split(service.Endpoint.VirtualIPs[0].Addr,"/")[0]
		}
	}
	return "ERR"
}
func getEndpointFromContainerId(container_id string) string {
	container_info, err := docker_cli.ContainerInspect(context.Background(), container_id)
	if err != nil {
		panic(err)
	}
	for _, net_value := range (container_info.NetworkSettings.Networks) {
		fmt.Println("EndpointId : ", net_value.EndpointID)	
		return net_value.EndpointID
	}
	return "ERR"
}
func getAllContainersIpInService(service_name string) map[string]string {
	container_map := make(map[string]string)
	containers, err := docker_cli.ContainerList(context.Background(), docker_types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	for _, container := range containers {
		//spew.Dump(container.NetworkSettings.Networks)
		for label, value := range container.Labels {
	 		if (label == "com.docker.swarm.service.name") {
				if strings.Compare(service_name,value) == 0 {
					//container_map[container.ID] = container.NetworkSettings.Networks		
					for _, net_value := range (container.NetworkSettings.Networks) {
						fmt.Println("LB IP ADDR : ", net_value.IPAddress)	
						container_map[container.ID] = net_value.IPAddress
						break
					}
				}
			}
		}
	}
	return container_map
}

func getSubnetFromNetwork(networkObj *types.VirtualNetwork) string {
	net_ipams, _ := networkObj.GetNetworkIpamRefs()
	for _, net_ipam := range net_ipams {
		fmt.Printf("Subnet uuid: %s\n", net_ipam.Attr.(types.VnSubnetsType).IpamSubnets[0].SubnetUuid)
		return net_ipam.Attr.(types.VnSubnetsType).IpamSubnets[0].SubnetUuid
	}
	return "ERR"
}

func addFloatingIp(vmi_uuid string, fip_pool_arg string) {
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
	interface_obj, err := oc_client.FindByUuid("virtual-machine-interface", vmi_uuid)
	if err != nil {
		fmt.Printf("Error in finding interface obj\n")
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	fip_arr := strings.Split(fip_pool_arg, ":")
    	public_network, fip_pool, ip_addr := fip_arr[0], fip_arr[1], fip_arr[2]
	fmt.Println(public_network, fip_pool, ip_addr)
	var fqn_fippool []string
	fqn_fippool = append(fqn_fippool, "default-domain")
	fqn_fippool = append(fqn_fippool, os_tenant_name)
	fqn_fippool = append(fqn_fippool, public_network)
	fqn_fippool = append(fqn_fippool, fip_pool)
	fmt.Println(strings.Join(fqn_fippool, ":"))
	fip_pool_obj, err := types.FloatingIpPoolByName(oc_client , strings.Join(fqn_fippool, ":"))
	if err != nil {
		fmt.Printf("Error in finding fip pool obj\n")
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	floating_obj := new(types.FloatingIp)
	floating_obj.SetParent(fip_pool_obj)
	floating_obj.AddProject(project_obj.(*types.Project))
	floating_obj.SetFloatingIpAddress(ip_addr)
	floating_obj.AddVirtualMachineInterface(interface_obj.(*types.VirtualMachineInterface))
	floating_obj.SetName(randomdata.SillyName())
	floating_obj.SetFloatingIpIsVirtualIp(true)
	err = oc_client.Create(floating_obj)
	if err != nil {
		fmt.Printf("Error in creating floating ip instance\n")
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	
		
}

func main() {
	InitFlags()
	flag.Usage = usage
	flag.Parse()

	/*
	if flag.NArg() == 0 {
		usage()
		os.Exit(2)
	}
	*/

	oc_client = contrail.NewClient(oc_server, oc_port)
	//networkList(oc_client)

	fmt.Println("Loadbalancing service name: ", service_name)
	var err error
	docker_cli, err = docker_client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	//fmt.Printf("^^^^^^^^^^^^^ %s\n", getDockerServiceNetwork(service_name))
	//getAllContainersIpInService(service_name)
	//getDockerServiceNetwork(service_name)
	if delete_lb == true {	
		DeleteLoadBalancer(service_name)
		return
	}
	//addFloatingIp("19fb9b9e-3ace-4d00-9606-9bc9e3d040ca", fip_pool_arg)
	CreateLoadBalancer(service_name)
	//getEndpointFromContainerId("ce74eed0bc2b466503e72b41372b3525438f0c23deae6f43870248929e978f56")
	
}
