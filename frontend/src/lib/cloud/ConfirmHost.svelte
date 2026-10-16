<script lang="ts">
 import * as Dialog from '$lib/components/ui/dialog';import {Button} from '$lib/components/ui/button';import {confirmation,finishConfirmation} from './confirmation.svelte';
</script>
<Dialog.Root open={confirmation.open} onOpenChange={open=>{if(!open)finishConfirmation(false);}}><Dialog.Content class="sm:max-w-md"><Dialog.Header><Dialog.Title>Discard unsaved changes?</Dialog.Title><Dialog.Description>{confirmation.message} Your last saved version will remain available.</Dialog.Description></Dialog.Header><Dialog.Footer><Button variant="outline" onclick={()=>finishConfirmation(false)}>Keep editing</Button><Button variant="destructive" onclick={()=>finishConfirmation(true)}>Discard changes</Button></Dialog.Footer></Dialog.Content></Dialog.Root>
