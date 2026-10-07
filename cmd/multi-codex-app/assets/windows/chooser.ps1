param([Parameter(Mandatory=$true)][string]$ConfigPath)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName PresentationFramework
$config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json
[xml]$layout = @'
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation" Title="Multi Codex" Width="480" Height="500" MinHeight="350" WindowStartupLocation="CenterScreen" Background="#F5F6F9">
 <Grid Margin="28"><Grid.RowDefinitions><RowDefinition Height="Auto"/><RowDefinition Height="Auto"/><RowDefinition Height="*"/><RowDefinition Height="Auto"/></Grid.RowDefinitions>
  <TextBlock Text="Choose a Codex profile" FontSize="24" FontWeight="SemiBold" Foreground="#191D2A"/>
  <TextBlock Grid.Row="1" Text="Use the profile where you clicked Connect." Margin="0,10,0,22" Foreground="#626A7D"/>
  <ListBox Name="Profiles" Grid.Row="2" BorderThickness="0" Background="White" Padding="8" FontSize="16" DisplayMemberPath="name"/>
  <StackPanel Grid.Row="3" Orientation="Horizontal" HorizontalAlignment="Right" Margin="0,22,0,0"><Button Name="Cancel" Content="Cancel" Width="85" Padding="8" Margin="0,0,10,0" IsCancel="True"/><Button Name="Continue" Content="Continue" Width="100" Padding="8" IsEnabled="False" IsDefault="True"/></StackPanel>
 </Grid>
</Window>
'@
$window = [Windows.Markup.XamlReader]::Load((New-Object System.Xml.XmlNodeReader $layout))
$list = $window.FindName('Profiles')
foreach ($profile in $config.profiles) { [void]$list.Items.Add($profile) }
$continue = $window.FindName('Continue')
$list.Add_SelectionChanged({ $continue.IsEnabled = $null -ne $list.SelectedItem })
$script:choice = $null
$continue.Add_Click({ $script:choice = $list.SelectedItem.id; $window.Close() })
$window.FindName('Cancel').Add_Click({ $window.Close() })
[void]$window.ShowDialog()
if ($null -ne $script:choice) { [Console]::Out.WriteLine($script:choice) }
