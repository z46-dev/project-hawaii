import { Card, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

type DashboardCardProps = { title: string; description: string };

export function DashboardCard({ title, description }: DashboardCardProps) {
    return (
        <Card className="min-h-[180px]">
            <CardHeader>
                <CardTitle>{title}</CardTitle>
                <CardDescription>{description}</CardDescription>
            </CardHeader>
            <CardFooter className="mt-auto border-0 bg-transparent px-4 pt-0">
                <Badge variant="secondary">Not connected</Badge>
            </CardFooter>
        </Card>
    );
}
