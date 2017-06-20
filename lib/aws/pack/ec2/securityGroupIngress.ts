// *** WARNING: this file was generated by the Lumi IDL Compiler (LUMIDL). ***
// *** Do not edit by hand unless you're certain you know what you are doing! ***

/* tslint:disable:ordered-imports variable-name */
import * as lumi from "@lumi/lumi";

import {SecurityGroup} from "./securityGroup";

export class SecurityGroupIngress extends lumi.NamedResource implements SecurityGroupIngressArgs {
    public readonly ipProtocol: string;
    public readonly cidrIp?: string;
    public readonly cidrIpv6?: string;
    public readonly fromPort?: number;
    public readonly group?: SecurityGroup;
    public readonly groupName?: string;
    public readonly sourceSecurityGroup?: SecurityGroup;
    public readonly sourceSecurityGroupName?: string;
    public readonly sourceSecurityGroupOwnerId?: string;
    public readonly toPort?: number;

    constructor(name: string, args: SecurityGroupIngressArgs) {
        super(name);
        if (args.ipProtocol === undefined) {
            throw new Error("Missing required argument 'ipProtocol'");
        }
        this.ipProtocol = args.ipProtocol;
        this.cidrIp = args.cidrIp;
        this.cidrIpv6 = args.cidrIpv6;
        this.fromPort = args.fromPort;
        this.group = args.group;
        this.groupName = args.groupName;
        this.sourceSecurityGroup = args.sourceSecurityGroup;
        this.sourceSecurityGroupName = args.sourceSecurityGroupName;
        this.sourceSecurityGroupOwnerId = args.sourceSecurityGroupOwnerId;
        this.toPort = args.toPort;
    }

    public static get(id: lumi.ID): SecurityGroupIngress {
        return <any>undefined; // functionality provided by the runtime
    }

    public static query(q: any): SecurityGroupIngress[] {
        return <any>undefined; // functionality provided by the runtime
    }
}

export interface SecurityGroupIngressArgs {
    readonly ipProtocol: string;
    readonly cidrIp?: string;
    readonly cidrIpv6?: string;
    readonly fromPort?: number;
    readonly group?: SecurityGroup;
    readonly groupName?: string;
    readonly sourceSecurityGroup?: SecurityGroup;
    readonly sourceSecurityGroupName?: string;
    readonly sourceSecurityGroupOwnerId?: string;
    readonly toPort?: number;
}


